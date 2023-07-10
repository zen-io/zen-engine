package cache

import (
	"os"
	"path/filepath"
)

func NewLocalCache() *LocalCache {
	return &LocalCache{}
}

type LocalCache struct {
	outpath string
}

func (lc *LocalCache) Load(cfg map[string]string) error {
	lc.outpath = cfg["out"]
	return nil
}

func (lc *LocalCache) Save(key, fpath string) func() error {
	return func() error {
		return nil
	}
}

func (lc *LocalCache) Restore(key string) func() error {
	return func() error {
		return nil
	}
}

func (lc *LocalCache) Delete(key string) func() error {
	return func() error {
		return nil
	}
}

func (lc *LocalCache) CheckOutputsExist(key string) func() (bool, error) {
	return func() (bool, error) {
		if _, err := os.Stat(filepath.Join(lc.outpath, key)); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			} else {
				return false, err
			}
		}

		return true, nil
	}
}
