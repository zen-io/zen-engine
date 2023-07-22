package parser

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/zen-io/zen-core/utils"
	eng_utils "github.com/zen-io/zen-engine/utils"
)

type ReadRequest struct {
	Blocks map[string][]map[string]interface{}
	Vars   map[string]string
}

func (rr *ReadRequest) ReadPackageFile(path string) error {
	pkgBlocks, err := eng_utils.ReadHclFile(path)
	if err != nil {
		return err
	}

	if varBlocks, ok := pkgBlocks["variables"]; ok {
		for _, block := range varBlocks {
			for k, v := range block {
				rr.Vars[strings.ToUpper(k)] = v.(string)
			}
		}

		delete(pkgBlocks, "variables")
	}

	if includedBlocks, ok := pkgBlocks["include"]; ok {
		err := rr.parseIncludeBlocks(path, includedBlocks)
		if err != nil {
			return fmt.Errorf("parsing included blocks: %w", err)
		}

		delete(pkgBlocks, "include")
	}

	for blockType, blocks := range pkgBlocks {
		if rr.Blocks[blockType] == nil {
			rr.Blocks[blockType] = make([]map[string]interface{}, 0)
		}
		rr.Blocks[blockType] = append(rr.Blocks[blockType], blocks...)
	}

	return err
}

func (rr ReadRequest) parseIncludeBlocks(path string, blocks []map[string]interface{}) error {
	for _, b := range blocks {
		var ic IncludeConfig
		mapstructure.Decode(b, &ic)
		ic.Variables = rr.Vars

		if ic.Path == ic.Template && ic.Path == nil {
			return fmt.Errorf("path or template are needed when including")
		}
		if ic.Path != nil {
			ic.Path = utils.StringPtr(filepath.Join(filepath.Dir(path), *ic.Path))
			err := ic.getFromPath(rr.ReadPackageFile)
			if err != nil {
				return err
			}
		}

		if ic.Template != nil {
			if !strings.HasPrefix(*ic.Template, "/") {
				ic.Template = utils.StringPtr(filepath.Join(filepath.Dir(path), *ic.Template))
			}

			included, err := ic.getFromTemplate()
			if err != nil {
				return err
			}

			for blockType, blocks := range included {
				if rr.Blocks[blockType] == nil {
					rr.Blocks[blockType] = make([]map[string]interface{}, 0)
				}
				rr.Blocks[blockType] = append(rr.Blocks[blockType], blocks...)
			}
		}
	}

	return nil
}
