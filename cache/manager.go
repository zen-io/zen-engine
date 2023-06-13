package cache

import (
	"fmt"
	"path/filepath"
	"strings"

	zen_target "github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"
	"github.com/zen-io/zen-engine/config"

	atomics "github.com/tiagoposse/go-sync-types"
)

type CacheManager struct {
	config *config.CacheConfig
	items  *atomics.Map[string, *CacheItem] //map[string]*CacheItem
}

func NewCacheManager(config *config.CacheConfig) *CacheManager {
	cm := &CacheManager{
		config: config,
		items:  atomics.NewMap[string, *CacheItem](),
	}
	return cm
}

func (cm *CacheManager) LoadTargetCache(target *zen_target.Target) (*CacheItem, error) {
	buildStepFqn := fmt.Sprintf("%s:build", target.Qn())
	if val, ok := cm.items.Get(buildStepFqn); ok {
		return val, nil
	}

	pkgPath := strings.ReplaceAll(target.Qn(), ":", "/")

	cacheItem := &CacheItem{
		target: target,
		Mappings: &CacheItemMappings{
			Srcs: make(map[string]map[string]string),
			Outs: make(map[string]string),
		},
	}
	if !target.External {
		cacheItem.BaseBuildCache = filepath.Join(*cm.config.Tmp, pkgPath)
		cacheItem.OutDest = filepath.Join(*cm.config.Out, pkgPath)
	}

	srcHashes, err := cm.MapTargetSrcs(cacheItem)
	if err != nil {
		return nil, err
	}

	if err := cacheItem.CalculateTargetBuildHash(srcHashes); err != nil {
		return nil, err
	}

	if err := cacheItem.ExpandSrcs(); err != nil {
		return nil, err
	}

	cacheItem.MetadataPath = filepath.Join(*cm.config.Metadata, pkgPath, cacheItem.Hash, "metadata.json")

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
					return nil, err
				} else {
					mappings[srcCategory] = utils.MergeMaps(mappings[srcCategory], m)

					hashes[srcCategory][src], err = cm.TargetHash(src)
					if err != nil {
						return nil, err
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
							return nil, err
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
					return nil, err
				} else {
					hashes[srcCategory][src] = h
				}
			}
		}

	}

	ci.Mappings.Srcs = mappings

	return hashes, nil
}
