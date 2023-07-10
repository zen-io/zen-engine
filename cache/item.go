package cache

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"
)

type CacheItemMappings struct {
	Srcs map[string]map[string]string
	Outs map[string]string
}

type CacheItem struct {
	target *target.Target

	Hash              string
	BaseBuildCache    string
	MetadataPath      string
	OutDest           string
	Save              func() error
	Restore           func() error
	CheckOutputsExist func() (bool, error)

	Mappings *CacheItemMappings
}

func (ci *CacheItem) BuildCachePath() string {
	if ci.target.External || ci.BaseBuildCache == "" {
		return ci.target.Path()
	} else {
		return filepath.Join(ci.BaseBuildCache, ci.Hash)
	}
}

func (ci *CacheItem) BuildOutPath() string {
	if ci.OutDest == "" {
		return ci.target.Path()
	} else {
		return filepath.Join(ci.OutDest)
	}
}

// Requires cache to be computed
func (ci *CacheItem) CheckCacheHits() bool {
	_, err := os.Stat(ci.MetadataPath)
	return err == nil
}

func (ci *CacheItem) CheckDeployed() bool {
	return false
}

func (ci *CacheItem) SaveMetadata() error {
	if err := os.MkdirAll(filepath.Dir(ci.MetadataPath), os.ModePerm); err != nil {
		return fmt.Errorf("creating metadata folder: %w", err)
	}

	if data, err := json.Marshal(map[string]string{}); err != nil {
		return err
	} else if err := os.WriteFile(ci.MetadataPath, data, 0644); err != nil {
		return err
	}

	return nil
}

func (ci *CacheItem) DeleteCache() error {
	if err := os.RemoveAll(filepath.Dir(ci.MetadataPath)); err != nil {
		return fmt.Errorf("clean metadata: %w", err)
	}

	if !ci.target.External {
		if err := os.RemoveAll(ci.BuildCachePath()); err != nil {
			return fmt.Errorf("clean build: %w", err)
		}
	}

	if err := os.RemoveAll(ci.BuildOutPath()); err != nil {
		return fmt.Errorf("clean outs: %w", err)
	}

	return nil
}

func (ci *CacheItem) VerifyOutputs(outs []string) (bool, error) {
	for _, out := range outs {
		if strings.Contains(out, "*") {
			if err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
				return filepath.SkipAll
			}); errors.Is(err, filepath.SkipAll) {
				continue
			} else {
				return false, err
			}
		} else {
			if _, err := os.Stat(out); os.IsNotExist(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("error checking output %s: %w", out, err)
			}
		}
	}

	return true, nil
}

func (ci *CacheItem) CopySrcsToCache() error {
	if ci.BaseBuildCache == "" || ci.target.External {
		return nil
	}

	for _, srcMap := range ci.Mappings.Srcs {
		for srcName, srcPath := range srcMap {
			from := filepath.Join(srcPath)
			to := filepath.Join(ci.BuildCachePath(), srcName)

			ci.target.Traceln("into cache: from \"%s\" to \"%s\"", from, to)
			if err := utils.Copy(from, to); err != nil {
				return fmt.Errorf("copying src to cache: %w", err)
			}
		}
	}

	return nil
}

func (ci *CacheItem) CopyOutsIntoOut() error {
	if err := os.RemoveAll(ci.BuildOutPath()); err != nil {
		return fmt.Errorf("removing preexisting out dir: %w", err)
	}

	// Create the out dir
	if err := os.MkdirAll(ci.BuildOutPath(), os.ModePerm); err != nil {
		return fmt.Errorf("creating out dir: %s", err)
	}

	if ci.OutDest == "" {
		return nil
	}

	for to, from := range ci.Mappings.Outs {
		// from := filepath.Join(ci.BuildCachePath(), fromBase)
		to = filepath.Join(ci.BuildOutPath(), to)
		// fmt.Printf("into out: from %s to %s\n", from, to)

		if err := os.MkdirAll(filepath.Dir(to), os.ModePerm); err != nil {
			return fmt.Errorf("creating directory for output: %w", err)
		}

		ci.target.Traceln("Out from %s to %s", from, to)
		if err := utils.Copy(from, to); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("out %s does not exist: %w", from, err)
			} else {
				return fmt.Errorf("copying out: %w", err)
			}
		}
	}

	return nil
}

func (ci *CacheItem) ExportOutsToPath() error {
	for _, toBase := range ci.Mappings.Outs {
		from := filepath.Join(ci.BuildOutPath(), toBase)
		to := filepath.Join(ci.target.Path(), toBase)

		if err := utils.Copy(from, to); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("out %s does not exist", toBase)
			} else {
				return fmt.Errorf("copying out: %w", err)
			}
		}
	}

	return nil
}

func (ci *CacheItem) ExpandOuts(outs []string) error {
	expanded := []string{}
	for _, o := range outs {
		if strings.Contains(o, "*") {
			m, err := utils.GlobPath(
				ci.BuildCachePath(),
				o,
			)

			if err != nil {
				return err
			}

			ci.Mappings.Outs = utils.MergeMaps(ci.Mappings.Outs, m)
			for k, v := range m {
				if transformed, add := ci.target.TransformOut(ci.target, v); add {
					ci.Mappings.Outs[k] = transformed
					expanded = append(expanded, filepath.Join(ci.BuildOutPath(), k))
				}
			}
		} else {
			if transformed, add := ci.target.TransformOut(ci.target, o); add {
				ci.Mappings.Outs[o] = filepath.Join(ci.BuildCachePath(), transformed)
				expanded = append(expanded, filepath.Join(ci.BuildOutPath(), o))
			}
		}
	}

	ci.target.Outs = expanded

	return nil
}

func (ci *CacheItem) ExpandSrcs() error {
	expanded := map[string][]string{}
	for cat, srcs := range ci.Mappings.Srcs {
		expanded[cat] = make([]string, 0)
		for src := range srcs {
			expanded[cat] = append(expanded[cat], filepath.Join(ci.BuildCachePath(), src))
		}
	}

	ci.target.Srcs = expanded

	return nil
}

func (ci *CacheItem) CalculateTargetBuildHash(srcHashes map[string]map[string]string) error {
	shaHash := sha256.New()

	if _, err := shaHash.Write([]byte(fmt.Sprint(srcHashes))); err != nil {
		return err
	}

	if _, err := shaHash.Write([]byte(fmt.Sprint(ci.target.Env))); err != nil {
		return err
	}

	for _, label := range ci.target.Labels {
		if _, err := shaHash.Write([]byte(label)); err != nil {
			return err
		}
	}

	for e := range ci.target.Environments {
		if _, err := shaHash.Write([]byte(e)); err != nil {
			return err
		}
	}

	ci.Hash = fmt.Sprintf("%x", shaHash.Sum(nil))

	return nil
}

func (ci *CacheItem) Compress(out string) error {
	// create the output file
	outFile, err := os.Create(out)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// create the zstd writer
	zstdWriter, err := zstd.NewWriter(outFile)
	if err != nil {
		return err
	}

	// create a tar writer to write multiple files to the zstd writer
	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	// walk through the source directory and add all files to the tar writer
	err = filepath.Walk(ci.OutDest, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relPath, err := filepath.Rel(ci.OutDest, path)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		header := &tar.Header{
			Name: relPath,
			Mode: int64(info.Mode()),
			Size: info.Size(),
		}
		err = tarWriter.WriteHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(tarWriter, file)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// flush and close the zstd writer
	if err := zstdWriter.Close(); err != nil {
		return err
	}

	return nil
}

func (ci *CacheItem) Decompress(src string) error {
	// open the compressed file
	inFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer inFile.Close()

	// create the zstd reader
	zstdReader, err := zstd.NewReader(inFile)
	if err != nil {
		return err
	}

	// create a tar reader to read multiple files from the zstd reader
	tarReader := tar.NewReader(zstdReader)

	// extract each file from the tar reader
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		relPath := header.Name
		absPath := filepath.Join(ci.OutDest, relPath)
		if header.FileInfo().IsDir() {
			err = os.MkdirAll(absPath, header.FileInfo().Mode())
			if err != nil {
				return err
			}
		} else {
			file, err := os.Create(absPath)
			if err != nil {
				return err
			}

			defer file.Close()
			_, err = io.Copy(file, tarReader)
			if err != nil {
				return err
			}
			err = file.Chmod(header.FileInfo().Mode())
			if err != nil {
				return err
			}
		}
	}

	// close the zstd reader
	zstdReader.Close()

	return nil
}
