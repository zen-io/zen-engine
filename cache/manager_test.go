package cache

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zen-io/zen-core/mock"
	"github.com/zen-io/zen-core/utils"
	"gotest.tools/v3/assert"
)

func MockNewCacheManager(t *testing.T) *CacheManager {
	root := t.TempDir()

	return NewCacheManager(&CacheConfig{
		Gen:      utils.StringPtr(filepath.Join(root, "gen")),
		Metadata: utils.StringPtr(filepath.Join(root, "metadata")),
		Out:      utils.StringPtr(filepath.Join(root, "out")),
		Exec:     utils.StringPtr(filepath.Join(root, "exec")),
	})
}

func TestLoadTargetCacheSimple(t *testing.T) {
	cache := MockNewCacheManager(t)
	target := mock.MockBasicTarget(t)

	pkgPath := strings.ReplaceAll(target.Qn(), ":", "/")

	srcMappings := mock.MockSrcs["basic"].ExpandSrcsMappings(target.Path())
	mock.CreateFiles(t, mock.FileMapToSlice(t, srcMappings))

	ci, err := cache.LoadTargetCache(target)
	assert.NilError(t, err)
	assert.Equal(t, ci.BuildCachePath(), filepath.Join(*cache.config.Gen, pkgPath, ci.Hash))
	assert.Equal(t, ci.BuildOutPath(), filepath.Join(*cache.config.Out, pkgPath))
	assert.DeepEqual(t, ci.Mappings.Srcs, srcMappings)

	outMappings := mock.MockSrcs["basic"].ExpandOutsMappings(ci.BuildCachePath())
	ci.ExpandOuts(ci.target.Outs)
	assert.DeepEqual(t, ci.Mappings.Outs, outMappings)
}

// func TestLoadTargetCacheComplex(t *testing.T) {
// 	cache := MockNewCacheManager(t)
// 	basic := mock.MockBasicTarget(t)

// 	// pkgPath := strings.ReplaceAll(basic.Qn(), ":", "/")

// 	srcMappings := mock.MockSrcs["basic"].ExpandSrcsMappings(basic.Path())
// 	mock.CreateFiles(t, mock.FileMapToSlice(t, srcMappings))

// 	ci, _ := cache.LoadTargetCache(basic)
// 	ci.ExpandOuts(ci.target.Outs)

// 	complex = mock.MockComplextTarget(t)
// }
