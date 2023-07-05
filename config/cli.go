package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"

	environs "github.com/zen-io/zen-core/environments"

	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
	hclconv "github.com/tmccombs/hcl2json/convert"
)

type GlobalConfig struct {
	Projects map[string]string `hcl:"projects"`
}

type CacheConfig struct {
	Tmp      *string `hcl:"tmp"`
	Metadata *string `hcl:"metadata"`
	Out      *string `hcl:"out"`
	Exec     *string `hcl:"logs"`
	Exports  *string `hcl:"exports"`
}

type ParseConfig struct{}

type BuildConfig struct {
	PassEnv   []string          `hcl:"pass_env"`
	Variables map[string]string `hcl:"variables"`
	Path      *string           `hcl:"path"` // additional PATH
}

type DeployConfig struct {
	PassEnv   []string          `hcl:"pass_env"`
	Variables map[string]string `hcl:"variables"`
}

type HostConfig struct {
	OS   string
	Arch string
}

type Config struct {
	Global       *GlobalConfig `hcl:"global,block"`
	Parse        *ParseConfig  `hcl:"parse,block"`
	Cache        *CacheConfig  `hcl:"cache,block"`
	Build        *BuildConfig  `hcl:"build,block"`
	Deploy       *DeployConfig `hcl:"deploy,block"`
	Host         *HostConfig
	Environments map[string]*environs.Environment `hcl:"environments,block"`
}

func LoadConfig() (*Config, error) {
	baseCfg := &Config{
		Global: &GlobalConfig{
			Projects: map[string]string{},
		},
		Cache: &CacheConfig{
			Tmp:      StringPtr(os.Getenv("HOME") + "/.zen/cache"),
			Out:      StringPtr(os.Getenv("HOME") + "/.zen/out"),
			Metadata: StringPtr(os.Getenv("HOME") + "/.zen/metadata"),
			Exec:     StringPtr(os.Getenv("HOME") + "/.zen/exec"),
			Exports:  StringPtr(os.Getenv("HOME") + "/.zen/exports"),
		},
		Parse:        &ParseConfig{},
		Environments: map[string]*environs.Environment{},
		Build:        &BuildConfig{},
		Deploy:       &DeployConfig{},
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

	loadedCfg := &Config{}
	if val, ok := unmarshalledConfig["parse"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Parse block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Parse)
		}
	}

	if val, ok := unmarshalledConfig["cache"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Cache block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Cache)
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

	if val, ok := unmarshalledConfig["deploy"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Deploy block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Deploy)
		}
	}

	if loadedCfg.Parse == nil {
		loadedCfg.Parse = &ParseConfig{}
	}

	if loadedCfg.Cache == nil {
		loadedCfg.Cache = &CacheConfig{}
	}

	if loadedCfg.Build == nil {
		loadedCfg.Build = &BuildConfig{}
	}

	if loadedCfg.Deploy == nil {
		loadedCfg.Deploy = &DeployConfig{}
	}

	mergo.Merge(baseCfg.Cache, loadedCfg.Cache, mergo.WithOverride)
	mergo.Merge(baseCfg.Parse, loadedCfg.Parse, mergo.WithOverride)
	mergo.Merge(baseCfg.Global, loadedCfg.Global, mergo.WithOverride)
	mergo.Merge(baseCfg.Build, loadedCfg.Build, mergo.WithOverride)
	mergo.Merge(baseCfg.Deploy, loadedCfg.Deploy, mergo.WithOverride)
	baseCfg.Environments = environs.MergeEnvironmentMaps(baseCfg.Environments, loadedCfg.Environments)

	return baseCfg, nil
}

func StringPtr(str string) *string {
	return &str
}
