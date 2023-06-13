package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	environs "github.com/baulos-io/baulos-core/environments"

	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
	hclconv "github.com/tmccombs/hcl2json/convert"
)

type ProjectConfig struct {
	Baulos   *ProjectBauLosConfig    `mapstructure:"baulos"`
	Parse    *ProjectParseConfig     `mapstructure:"parse"`
	Build    *ProjectBuildConfig     `mapstructure:"build"`
	Deploy   *ProjectDeployConfig    `mapstructure:"deploy"`
	Plugins  []*ProjectPluginConfig  `mapstructure:"plugin"`
	Commands []*ProjectCommandConfig `mapstructure:"command"`
}

type ProjectBauLosConfig struct {
	Version string `mapstructure:"version"`
}

type ProjectParseConfig struct {
	Filename  string   `mapstructure:"filename"`
	Placement []string `mapstructure:"placement"`
}

type ProjectBuildConfig struct {
	Toolchains    map[string]string `mapstructure:"toolchains"`
	Variables     map[string]string `mapstructure:"variables"`
	PassEnv       []string          `mapstructure:"pass_env"`
	PassSecretEnv []string          `mapstructure:"pass_secret_env"`
}

type ProjectDeployConfig struct {
	Environments map[string]*environs.Environment `mapstructure:"environments"`
	Variables    map[string]string                `mapstructure:"variables"`
}

type ProjectPluginConfig struct {
	Name string
	Repo *string
	Path *string
}

type ProjectCommandConfig struct {
	Name string
	Repo *string
	Path *string
}

type Project struct {
	Path   string
	Config *ProjectConfig
}

func LoadProjectConfig(configPath string) (*ProjectConfig, error) {
	if _, err := os.Stat(configPath); err != nil {
		return nil, fmt.Errorf("config %s does not exist", configPath)
	}

	baseCfg := &ProjectConfig{
		Baulos: &ProjectBauLosConfig{
			Version: "latest",
		},
		Parse: &ProjectParseConfig{
			Filename:  "BUILD",
			Placement: []string{},
		},
		Build: &ProjectBuildConfig{
			Toolchains:    make(map[string]string),
			PassEnv:       make([]string, 0),
			Variables:     make(map[string]string),
			PassSecretEnv: make([]string, 0),
		},
		Deploy: &ProjectDeployConfig{
			Environments: map[string]*environs.Environment{},
			Variables:    make(map[string]string),
		},
		Plugins:  []*ProjectPluginConfig{},
		Commands: []*ProjectCommandConfig{},
	}

	var content, jsonBytes []byte
	var err error
	if content, err = ioutil.ReadFile(configPath); err != nil {
		return nil, err
	}

	if jsonBytes, err = hclconv.Bytes(content, configPath, hclconv.Options{
		Simplify: false,
	}); err != nil {
		return nil, err
	}

	var unmarshalledConfig map[string][]interface{}
	if err := json.Unmarshal(jsonBytes, &unmarshalledConfig); err != nil {
		return nil, err
	}

	loadedCfg := &ProjectConfig{}
	if val, ok := unmarshalledConfig["parse"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Parse block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Parse)
		}
	} else {
		loadedCfg.Parse = &ProjectParseConfig{}
	}
	if len(loadedCfg.Parse.Placement) == 0 {
		loadedCfg.Parse.Placement = []string{"{PKG}"}
	}

	if val, ok := unmarshalledConfig["baulos"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one config block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Baulos)
		}
	} else {
		loadedCfg.Baulos = &ProjectBauLosConfig{}
	}

	if val, ok := unmarshalledConfig["build"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one build block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Build)
		}
	} else {
		loadedCfg.Build = &ProjectBuildConfig{}
	}

	if val, ok := unmarshalledConfig["deploy"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one deploy block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Deploy)
		}
	} else {
		loadedCfg.Deploy = &ProjectDeployConfig{}
	}

	if val, ok := unmarshalledConfig["plugin"]; ok {
		err := mapstructure.Decode(val, &loadedCfg.Plugins)
		if err != nil {
			panic(err)
		}
	} else {
		loadedCfg.Plugins = []*ProjectPluginConfig{}
	}

	if val, ok := unmarshalledConfig["command"]; ok {
		err := mapstructure.Decode(val, &loadedCfg.Plugins)
		if err != nil {
			panic(err)
		}
	} else {
		loadedCfg.Plugins = []*ProjectPluginConfig{}
	}

	mergo.Merge(baseCfg, loadedCfg, mergo.WithOverride, mergo.WithAppendSlice)

	return baseCfg, nil
}
