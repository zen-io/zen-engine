package cache

import (
	"fmt"
	"path/filepath"
	"strings"

	zen_target "github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"

	atomics "github.com/tiagoposse/go-sync-types"
)

type CacheIO interface {
	Save(key, fpath string) func() error
	Restore(key string) func() error
	Delete(key string) func() error
	CheckOutputsExist(key string) func() (bool, error)
}

type CacheConfig struct {
	Tmp      *string           `hcl:"tmp"`
	Metadata *string           `hcl:"metadata"`
	Out      *string           `hcl:"out"`
	Exec     *string           `hcl:"logs"`
	Type     *string           `hcl:"type"`
	Config   map[string]string `hcl:"config"`
}

type CacheManager struct {
	config *CacheConfig
	io     CacheIO
	items  *atomics.Map[string, *CacheItem] //map[string]*CacheItem
}

func NewCacheManager(config *CacheConfig) *CacheManager {
	cm := &CacheManager{
		config: config,
		items:  atomics.NewMap[string, *CacheItem](),
	}

	// type defaults to local
	if *config.Type == "local" {
		cm.io = &LocalCache{
			outpath: *config.Out,
		}
	}
	return cm
}

func (cm *CacheManager) LoadTargetCache(target *zen_target.Target) (*CacheItem, error) {
	buildStepFqn := fmt.Sprintf("%s:build", target.Qn())
	if val, ok := cm.items.Get(buildStepFqn); ok {
		return val, nil
	}

	cacheItem := &CacheItem{
		target: target,
		Mappings: &CacheItemMappings{
			Srcs: make(map[string]map[string]string),
			Outs: make(map[string]string),
		},
	}

	if !target.External {
		cacheItem.BaseBuildCache = filepath.Join(*cm.config.Tmp, target.Package(), target.Name)
		cacheItem.OutDest = filepath.Join(*cm.config.Out, target.Package(), target.Name)
	}

	srcHashes, err := cm.MapTargetSrcs(cacheItem)
	if err != nil {
		return nil, fmt.Errorf("mapping srcs: %w", err)
	}

	if err := cacheItem.CalculateTargetBuildHash(srcHashes); err != nil {
		return nil, fmt.Errorf("calculating hash: %w", err)
	}

	if err := cacheItem.ExpandSrcs(); err != nil {
		return nil, fmt.Errorf("expanding srcs: %w", err)
	}

	cacheKey := filepath.Join(target.Package(), target.Name, cacheItem.Hash)
	cacheItem.MetadataPath = filepath.Join(*cm.config.Metadata, cacheKey+".json")
	cacheItem.Save = cm.io.Save(cacheKey, filepath.Join(*cm.config.Tmp, cacheKey+".tar"))
	cacheItem.Restore = cm.io.Restore(cacheKey)
	cacheItem.CheckOutputsExist = cm.io.CheckOutputsExist(cacheKey)
	cm.items.Put(buildStepFqn, cacheItem)

	return cacheItem, nil
}

func (cm *CacheManager) TargetHash(qn string) (string, error) {
	ci, ok := cm.items.Get(qn)
	if !ok {
		return "", fmt.Errorf("%s not found in cache", qn)
	}

	if ci.Hash == "" {
		return "", fmt.Errorf("cache for target %s not initialized", qn)
	}

	return ci.Hash, nil
}

func (cm *CacheManager) TargetOuts(stepQn string) (map[string]string, error) {
	ci, ok := cm.items.Get(stepQn)
	if !ok {
		return nil, fmt.Errorf("%s not in cache", stepQn)
	}

	m := map[string]string{}
	for _, o := range ci.target.Outs {
		m[strings.TrimPrefix(o, ci.BuildOutPath())] = o
	}

	return m, nil
}

func (cm *CacheManager) MapTargetSrcs(ci *CacheItem) (map[string]map[string]string, error) {
	mappings := make(map[string]map[string]string)
	hashes := make(map[string]map[string]string)

	for srcCategory, sSrcs := range ci.target.Srcs {
		mappings[srcCategory] = map[string]string{}
		hashes[srcCategory] = map[string]string{}

		for _, src := range sSrcs {
			if zen_target.IsTargetReference(src) { // src is a reference
				if m, err := cm.TargetOuts(src); err != nil {
					return nil, fmt.Errorf("getting ref outs %s, %w", src, err)
				} else {
					mappings[srcCategory] = utils.MergeMaps(mappings[srcCategory], m)

					hashes[srcCategory][src], err = cm.TargetHash(src)
					if err != nil {
						return nil, fmt.Errorf("getting ref hash %s, %w", src, err)
					}
				}

				continue
			}

			if strings.Contains(src, "*") {
				if m, err := utils.GlobPath(ci.target.Path(), src); err != nil {
					return nil, err
				} else {
					mappings[srcCategory] = utils.MergeMaps(mappings[srcCategory], m)
					for k, v := range m {
						h, err := utils.FileHash(v)
						if err != nil {
							return nil, fmt.Errorf("glob file hash %s, %w", v, err)
						} else {
							hashes[srcCategory][k] = h
						}
					}
				}
			} else {
				fullpath := utils.AbsoluteFilePath(ci.target.Path(), src)
				mappings[srcCategory][src] = fullpath
				h, err := utils.FileHash(fullpath)
				if err != nil {
					return nil, fmt.Errorf("abs file hash %s, %w", fullpath, err)
				} else {
					hashes[srcCategory][src] = h
				}
			}
		}

	}

	ci.Mappings.Srcs = mappings

	return hashes, nil
}
