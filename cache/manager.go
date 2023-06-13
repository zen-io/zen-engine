package cache

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/baulos-io/baulos/src/config"

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

func (cm *CacheManager) LoadTargetCache(target *target.Target) (*CacheItem, error) {
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

	srcHashes, err := cm.ExpandTargetSrcs(cacheItem)
	if err != nil {
		return nil, err
	}

	if err := cacheItem.CalculateTargetBuildHash(srcHashes); err != nil {
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

	// outs := map[string]string{}
	// for _, v := range ci.Mappings.Outs {
	// 	outs[v] = filepath.Join(ci.OutDest, v)
	// }

	// return outs, nil
	return ci.Mappings.Outs, nil
}

func (cm *CacheManager) ExpandTargetSrcs(ci *CacheItem) (map[string]map[string]string, error) {
	expandedSrcs := make(map[string][]string)
	mappings := make(map[string]map[string]string)
	hashes := make(map[string]map[string]string)

	for srcCategory, sSrcs := range ci.target.Srcs {
		expandedSrcs[srcCategory] = []string{}
		mappings[srcCategory] = map[string]string{}
		hashes[srcCategory] = map[string]string{}

		for _, src := range sSrcs {
			if target.IsTargetReference(src) { // src is a reference
				if m, err := cm.TargetOuts(src); err != nil {
					return nil, err
				} else {
					mappings[srcCategory] = utils.MergeMaps(mappings[srcCategory], m)

					for _, v := range m {
						expandedSrcs[srcCategory] = append(expandedSrcs[srcCategory], v)
					}

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
						expandedSrcs[srcCategory] = append(expandedSrcs[srcCategory], v)

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
				expandedSrcs[srcCategory] = append(expandedSrcs[srcCategory], fullpath)
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
	ci.target.Srcs = expandedSrcs

	return hashes, nil
}
