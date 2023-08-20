package config

import (
	"fmt"
	"os"
	"runtime"

	environs "github.com/zen-io/zen-core/environments"
	"github.com/zen-io/zen-core/utils"
	zen_utils "github.com/zen-io/zen-engine/utils"

	"dario.cat/mergo"
	"github.com/mitchellh/mapstructure"
)

type GlobalConfig struct {
	Projects map[string]string `hcl:"projects"`
}

type ParseConfig struct{}

type BuildConfig struct {
	PassEnv       []string          `hcl:"pass_env"`
	Env           map[string]string `hcl:"env"`
	PassSecretEnv []string          `hcl:"pass_secret_env"`
	SecretEnv     map[string]string `hcl:"secret_env"`
	Path          *string           `hcl:"path"` // additional PATH
}

type HostConfig struct {
	OS   string
	Arch string
}

type CliConfig struct {
	Global       *GlobalConfig `hcl:"global,block"`
	Parse        *ParseConfig  `hcl:"parse,block"`
	Build        *BuildConfig  `hcl:"build,block"`
	Host         *HostConfig
	Environments map[string]*environs.Environment `hcl:"environments,block"`
}

func LoadConfig() (*CliConfig, error) {
	baseCfg := &CliConfig{
		Global: &GlobalConfig{
			Projects: map[string]string{},
		},
		Parse:        &ParseConfig{},
		Environments: map[string]*environs.Environment{},
		Build: &BuildConfig{
			PassEnv:       make([]string, 0),
			Env:           make(map[string]string),
			PassSecretEnv: make([]string, 0),
			SecretEnv:     map[string]string{
				"HOME": os.Getenv("HOME"),
			},
		},
		Host: &HostConfig{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
	}

	var configPath string
	if value, ok := os.LookupEnv("ZEN_CONFIG"); ok {
		configPath = value
	} else {
		configPath = os.Getenv("HOME") + "/.zen/conf.hcl"
	}

	unmarshalledConfig, err := zen_utils.ReadHclFile(configPath)
	if err != nil {
		return nil, err
	}

	loadedCfg := &CliConfig{}
	if val, ok := unmarshalledConfig["parse"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Parse block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Parse)
		}
	}

	if val, ok := unmarshalledConfig["global"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Global block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Global)
		}
	}

	if val, ok := unmarshalledConfig["environments"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Environments block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Environments)
		}
	}

	if val, ok := unmarshalledConfig["build"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Build block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Build)
		}
	}

	if loadedCfg.Parse == nil {
		loadedCfg.Parse = &ParseConfig{}
	}

	if loadedCfg.Build == nil {
		loadedCfg.Build = &BuildConfig{
			Path: utils.StringPtr(""),
		}
	}

	mergo.Merge(baseCfg.Parse, loadedCfg.Parse, mergo.WithOverride)
	mergo.Merge(baseCfg.Global, loadedCfg.Global, mergo.WithOverride)
	mergo.Merge(baseCfg.Build, loadedCfg.Build, mergo.WithOverride)
	baseCfg.Environments = environs.MergeEnvironmentMaps(baseCfg.Environments, loadedCfg.Environments)

	return baseCfg, nil
}

func StringPtr(str string) *string {
	return &str
}
