package parser

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/zen-io/zen-core/utils"
	eng_utils "github.com/zen-io/zen-engine/utils"
)

type IncludeConfig struct {
	Path      *string                `mapstructure:"path"`
	Template  *string                `mapstructure:"template"`
	Inputs    map[string]interface{} `mapstructure:"inputs"`
	Variables map[string]string
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

func (ic *IncludeConfig) getFromPath(getNextFile func(path string) error) error {
	interpolatedPath, err := utils.Interpolate(*ic.Path, ic.Variables)
	if err != nil {
		return fmt.Errorf("interpolating include path: %w", err)
	}

	err = getNextFile(interpolatedPath)
	if err != nil {
		return fmt.Errorf("including block in %s: %w", *ic.Path, err)
	}

	return nil
}

func (ic *IncludeConfig) getFromTemplate() (map[string][]map[string]interface{}, error) {
	interpolatedTemplatePath, err := utils.Interpolate(*ic.Template, ic.Variables)
	if err != nil {
		return nil, fmt.Errorf("interpolating template path: %w", err)
	}
	filename := filepath.Base(*ic.Template)

	readTemplate, err := eng_utils.ReadHclFile(interpolatedTemplatePath)
	if err != nil {
		return nil, err
	}

	it := &IncludeTemplate{}
	mapstructure.Decode(readTemplate["inputs"][0], &it.Inputs)
	mapstructure.Decode(readTemplate["template"][0], &it.Template)

	inputKeys := []string{}
	for inp, inpConf := range it.Inputs {
		if ic.Inputs[inp] == nil && inpConf.Mandatory {
			return nil, fmt.Errorf("input %s not provided but mandatory, at %s", inp, filename)
		}
		inputKeys = append(inputKeys, inp)
	}

	inputVars, err := ic.GetInputsAsVars(inputKeys)
	if err != nil {
		return nil, fmt.Errorf("computing input vars: %w", err)
	}
	inputVars, err = utils.InterpolateMap(inputVars, ic.Variables)
	if err != nil {
		return nil, fmt.Errorf("interpolating input vars: %w", err)
	}
	interpolatedContent := utils.InterpolateSetVars(it.Template, inputVars)

	includedBlocks, err := eng_utils.FromHclBytes([]byte(interpolatedContent), filename)
	if err != nil {
		return nil, err
	}

	return includedBlocks, nil
}
