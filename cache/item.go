package cache

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/baulos-io/baulos-core/target"
)

type CacheItemMappings struct {
	Srcs map[string]map[string]string
	Outs map[string]string
}

type CacheItem struct {
	target *target.Target

	Hash           string
	BaseBuildCache string
	MetadataPath   string
	OutDest        string

	Mappings *CacheItemMappings
}

func (ci *CacheItem) BuildCachePath() string {
	if ci.BaseBuildCache == "" {
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

	if err := os.RemoveAll(ci.BuildCachePath()); err != nil {
		return fmt.Errorf("clean build: %w", err)
	}

	if err := os.RemoveAll(ci.BuildOutPath()); err != nil {
		return fmt.Errorf("clean outs: %w", err)
	}

	return nil
}

func (ci *CacheItem) CheckOutputsExist(outs []string) (bool, error) {
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
	if ci.BaseBuildCache == "" {
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
		ci.target.Traceln("into out: from %s to %s", from, to)

		if err := os.MkdirAll(filepath.Dir(to), os.ModePerm); err != nil {
			return fmt.Errorf("creating directory for output: %w", err)
		}
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
			for _, v := range m {
				expanded = append(expanded, v)
			}
		} else {
			ci.Mappings.Outs[o] = filepath.Join(ci.BuildCachePath(), o)
			expanded = append(expanded, ci.Mappings.Outs[o])
		}
	}

	ci.target.Outs = expanded

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

	for e := range ci.target.Environments {
		if _, err := shaHash.Write([]byte(e)); err != nil {
			return err
		}
	}

	ci.Hash = fmt.Sprintf("%x", shaHash.Sum(nil))

	return nil
}
