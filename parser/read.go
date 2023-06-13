package parser

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/zen-io/zen-core/utils"

	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
	hclconv "github.com/tmccombs/hcl2json/convert"
)

type IncludeConfig struct {
	Path     *string                `mapstructure:"path"`
	Template *string                `mapstructure:"template"`
	Inputs   map[string]interface{} `mapstructure:"inputs"`
}

type IncludeInput struct {
	Mandatory bool `mapstructure:"mandatory"`
}

type IncludeTemplate struct {
	Inputs   map[string]IncludeInput `mapstructure:"inputs"`
	Template string                  `mapstructure:"template"`
}

func (ic *IncludeConfig) GetInputsAsVars(required []string) (map[string]string, error) {
	vars := map[string]string{}
	for k, v := range ic.Inputs {
		kind := reflect.TypeOf(v).Kind()
		if kind == reflect.Slice {
			quotedItems := []string{}
			for _, item := range v.([]interface{}) {
				quotedItems = append(quotedItems, fmt.Sprintf("\"%v\"", item))
			}
			vars[k] = strings.Join(quotedItems, ",")
		} else if kind == reflect.String {
			vars[k] = v.(string)
		} else if kind == reflect.Map {
			val, err := utils.SPrettyPrintFlatten(v)
			if err != nil {
				return nil, err
			}
			vars[k] = val
		}
	}

	for _, k := range required {
		if _, ok := vars[k]; !ok {
			vars[k] = ""
		}
	}
	return vars, nil
}

func (pp *PackageParser) ReadPackageFile(path, project, pkg string) (map[string][]map[string]interface{}, error) {
	var content, jsonBytes []byte
	var err error
	if content, err = ioutil.ReadFile(path); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if jsonBytes, err = hclconv.Bytes(content, path, hclconv.Options{
		Simplify: false,
	}); err != nil {
		return nil, fmt.Errorf("converting from hcl %s: %w", path, err)
	}

	var pkgBlocks map[string][]map[string]interface{}
	err = json.Unmarshal(jsonBytes, &pkgBlocks)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling blocks: %w", err)
	}

	vars := pp.contexts[project].Variables
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

			if ic.Path != nil {
				interpolatedPath, err := utils.Interpolate(*ic.Path, vars)
				if err != nil {
					return nil, fmt.Errorf("interpolating include path: %w", err)
				}
				if includedBlocks, err := pp.ReadPackageFile(filepath.Join(filepath.Dir(path), interpolatedPath), project, pkg); err != nil {
					return nil, fmt.Errorf("including in block %s: %w", path, err)
				} else {
					mergo.Merge(&pkgBlocks, includedBlocks, mergo.WithAppendSlice)
				}
			} else if ic.Template != nil {
				interpolatedTemplatePath, err := utils.Interpolate(*ic.Template, vars)
				if err != nil {
					utils.PrettyPrint(vars)
					return nil, fmt.Errorf("interpolating template path: %w", err)
				}

				content, err = ioutil.ReadFile(interpolatedTemplatePath)
				if err != nil {
					return nil, fmt.Errorf("reading interpolated template %s: %w", interpolatedTemplatePath, err)
				}

				if jsonBytes, err = hclconv.Bytes([]byte(content), path, hclconv.Options{
					Simplify: false,
				}); err != nil {
					return nil, fmt.Errorf("converting template file from hcl: %w", err)
				}
				var it *IncludeTemplate
				if err := json.Unmarshal(jsonBytes, &it); err != nil {
					return nil, fmt.Errorf("unmarshalling included template %s: %w", interpolatedTemplatePath, err)
				}

				inputKeys := []string{}
				for inp, inpConf := range it.Inputs {
					if ic.Inputs[inp] == nil && inpConf.Mandatory {
						return nil, fmt.Errorf("input %s not provided but mandatory, at %s", inp, path)
					}
					inputKeys = append(inputKeys, inp)
				}

				var includedBlocks map[string][]map[string]interface{}
				inputVars, err := ic.GetInputsAsVars(inputKeys)
				if err != nil {
					return nil, fmt.Errorf("computing input vars: %w", err)
				}
				inputVars, err = utils.InterpolateMap(inputVars, vars)
				if err != nil {
					return nil, fmt.Errorf("interpolating input vars: %w", err)
				}
				interpolatedContent := utils.InterpolateSetVars(it.Template, inputVars)

				if jsonBytes, err = hclconv.Bytes([]byte(interpolatedContent), path, hclconv.Options{
					Simplify: false,
				}); err != nil {
					return nil, fmt.Errorf("converting template from hcl: %w", err)
				}

				err = json.Unmarshal(jsonBytes, &includedBlocks)
				if err != nil {
					return nil, fmt.Errorf("unmarshalling included blocks in %s: %w", interpolatedTemplatePath, err)
				}

				mergo.Merge(&pkgBlocks, includedBlocks, mergo.WithAppendSlice)
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
