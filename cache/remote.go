package cache

// import (
// 	"archive/tar"
// 	"context"
// 	"io"
// 	"os"
// 	"path/filepath"

// 	"github.com/klauspost/compress/zstd"
// )

// func (ci *ChecksumCacheItem) compress() error {

// 	// create the output file
// 	outFile, err := os.Create(destFile)
// 	if err != nil {
// 		return err
// 	}
// 	defer outFile.Close()

// 	// create the zstd writer
// 	zstdWriter, err := zstd.NewWriter(outFile)
// 	if err != nil {
// 		return err
// 	}

// 	// create a tar writer to write multiple files to the zstd writer
// 	tarWriter := tar.NewWriter(zstdWriter)
// 	defer tarWriter.Close()

// 	// walk through the source directory and add all files to the tar writer
// 	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		if !info.Mode().IsRegular() {
// 			return nil
// 		}
// 		relPath, err := filepath.Rel(srcDir, path)
// 		if err != nil {
// 			return err
// 		}
// 		file, err := os.Open(path)
// 		if err != nil {
// 			return err
// 		}
// 		defer file.Close()
// 		header := &tar.Header{
// 			Name: relPath,
// 			Mode: int64(info.Mode()),
// 			Size: info.Size(),
// 		}
// 		err = tarWriter.WriteHeader(header)
// 		if err != nil {
// 			return err
// 		}
// 		_, err = io.Copy(tarWriter, file)
// 		if err != nil {
// 			return err
// 		}
// 		return nil
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	// flush and close the zstd writer
// 	if err := zstdWriter.Close(); err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (ci *ChecksumCacheItem) decompress(srcFile, destDir string) error {
// 	// open the compressed file
// 	inFile, err := os.Open(srcFile)
// 	if err != nil {
// 		return err
// 	}
// 	defer inFile.Close()

// 	// create the zstd reader
// 	zstdReader, err := zstd.NewReader(inFile)
// 	if err != nil {
// 		return err
// 	}

// 	// create a tar reader to read multiple files from the zstd reader
// 	tarReader := tar.NewReader(zstdReader)

// 	// extract each file from the tar reader
// 	for {
// 		header, err := tarReader.Next()
// 		if err == io.EOF {
// 			break
// 		}
// 		if err != nil {
// 			return err
// 		}
// 		relPath := header.Name
// 		absPath := filepath.Join(destDir, relPath)
// 		if header.FileInfo().IsDir() {
// 			err = os.MkdirAll(absPath, header.FileInfo().Mode())
// 			if err != nil {
// 				return err
// 			}
// 		} else {
// 			file, err := os.Create(absPath)
// 			if err != nil {
// 				return err
// 			}

// 			defer file.Close()
// 			_, err = io.Copy(file, tarReader)
// 			if err != nil {
// 				return err
// 			}
// 			err = file.Chmod(header.FileInfo().Mode())
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	// close the zstd reader
// 	zstdReader.Close()

// 	return nil
// }

// func (cm *CacheManager) upload(ci *ChecksumCacheItem) error {
// 	switch cm.config.Remote.Type {

// 	case DisabledRemoteCache:
// 		return nil
// 	case S3RemoteCache:
// 		if err := compress(); err != nil {
// 			return err
// 		}
// 		return s3Upload()
// 	case HttpRemoteCache:
// 		return nil
// 	case CommandRemoteCache:
// 		return nil
// 	}
// }

// func (cm *CacheManager) download(ci *ChecksumCacheItem) {

// }

// func s3Upload() {
// 	var file *os.File
// 	if f, err := os.Open(filepath.Join(target.Cwd, target.Srcs["_srcs"][0])); err != nil {
// 		return err
// 	} else {
// 		file = f
// 		defer file.Close()
// 	}

// 	cfgOpts := []func(*config.LoadOptions) error{}

// 	if val, ok := target.EnvVars()["AWS_PROFILE"]; ok {
// 		cfgOpts = append(cfgOpts,
// 			config.WithSharedConfigProfile(val),
// 			config.WithRegion(target.EnvVars()["AWS_REGION"]),
// 		)
// 	}
// 	// Create a new AWS configuration object using the given profile
// 	cfg, err := config.LoadDefaultConfig(context.TODO(), cfgOpts...)
// 	if err != nil {
// 		return err
// 	}
// 	client := s3.NewFromConfig(cfg)
// 	interpolatedBucket, err := target.Interpolate(fc.Bucket, runCtx, nil)
// 	if err != nil {
// 		return err
// 	}
// 	interpolatedBucketKey, err := target.Interpolate(fc.BucketKey, runCtx, nil)
// 	if err != nil {
// 		return err
// 	}

// 	// Upload the file to S3
// 	if _, err := client.PutObject(context.TODO(), &s3.PutObjectInput{
// 		Bucket: aws.String(interpolatedBucket),
// 		Key:    aws.String(interpolatedBucketKey),
// 		Body:   file,
// 	}); err != nil {
// 		return err
// 	}
// }
