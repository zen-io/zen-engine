package parser

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/zen-io/zen-core/utils"
	eng_utils "github.com/zen-io/zen-engine/utils"
)

func (pp *PackageParser) ReadPackageFile(path string, vars map[string]string) (map[string][]map[string]interface{}, error) {
	pkgBlocks, err := eng_utils.ReadHclFile(path)
	if err != nil {
		return nil, err
	}

	if blocks, ok := pkgBlocks["variables"]; ok {
		for k, v := range blocks[0] {
			vars[strings.ToUpper(k)] = v.(string)
		}
		delete(pkgBlocks, "variables")
	}

	if blocks, ok := pkgBlocks["include"]; ok {
		for _, b := range blocks {
			var ic IncludeConfig
			mapstructure.Decode(b, &ic)
			ic.Variables = vars

			if ic.Path == ic.Template && ic.Path == nil {
				return nil, fmt.Errorf("path or template are needed when including")
			}
			if ic.Path != nil {
				ic.Path = utils.StringPtr(filepath.Join(filepath.Dir(path), *ic.Path))
				included, err := ic.getFromPath(pp.ReadPackageFile, vars)
				if err != nil {
					return nil, err
				}

				for k, v := range included {
					pkgBlocks[k] = append(pkgBlocks[k], v...)
				}
			}

			if ic.Template != nil {
				if !strings.HasPrefix(*ic.Template, "/") {
					ic.Template = utils.StringPtr(filepath.Join(filepath.Dir(path), *ic.Template))
				}

				included, err := ic.getFromTemplate()
				if err != nil {
					return nil, err
				}
				for k, v := range included {
					pkgBlocks[k] = append(pkgBlocks[k], v...)
				}
			}
		}

		delete(pkgBlocks, "include")
	}

	blocksToReturn := make(map[string][]map[string]interface{})
	for blockType, blocks := range pkgBlocks {
		if blocksToReturn[blockType] == nil {
			blocksToReturn[blockType] = make([]map[string]interface{}, 0)
		}
		blocksToReturn[blockType] = append(blocksToReturn[blockType], blocks...)
	}

	return blocksToReturn, err
}
