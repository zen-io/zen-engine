package config

import (
	"fmt"
	"os"
	"runtime"

	environs "github.com/zen-io/zen-core/environments"
	"github.com/zen-io/zen-core/utils"
	zen_utils "github.com/zen-io/zen-engine/utils"

	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
)

type GlobalConfig struct {
	Projects map[string]string `hcl:"projects"`
}

type ParseConfig struct{}

type BuildConfig struct {
	PassEnv         []string          `hcl:"pass_env"`
	PassSecretEnv   []string          `hcl:"pass_env"`
	Variables       map[string]string `hcl:"variables"`
	SecretVariables map[string]string `hcl:"variables"`
	Path            *string           `hcl:"path"` // additional PATH
}

type DeployConfig struct {
	PassEnv   []string          `hcl:"pass_env"`
	Variables map[string]string `hcl:"variables"`
}

type HostConfig struct {
	OS   string
	Arch string
}

type CliConfig struct {
	Global       *GlobalConfig `hcl:"global,block"`
	Parse        *ParseConfig  `hcl:"parse,block"`
	Build        *BuildConfig  `hcl:"build,block"`
	Deploy       *DeployConfig `hcl:"deploy,block"`
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
			PassEnv: make([]string, 0),
			Variables: map[string]string{
				"USER":            os.Getenv("USER"),
				"HOME":            os.Getenv("HOME"),
				"SHLVL":           "1",
				"TARGET.OS":       runtime.GOOS,
				"TARGET.ARCH":     runtime.GOARCH,
				"CONFIG.HOSTOS":   runtime.GOOS,
				"CONFIG.HOSTARCH": runtime.GOARCH,
			},
		},
		Deploy: &DeployConfig{},
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

	if loadedCfg.Build == nil {
		loadedCfg.Build = &BuildConfig{}
	}

	if loadedCfg.Deploy == nil {
		loadedCfg.Deploy = &DeployConfig{}
	}

	mergo.Merge(baseCfg.Parse, loadedCfg.Parse, mergo.WithOverride)
	mergo.Merge(baseCfg.Global, loadedCfg.Global, mergo.WithOverride)
	mergo.Merge(baseCfg.Build, loadedCfg.Build, mergo.WithOverride)
	mergo.Merge(baseCfg.Deploy, loadedCfg.Deploy, mergo.WithOverride)
	baseCfg.Environments = environs.MergeEnvironmentMaps(baseCfg.Environments, loadedCfg.Environments)

	passedEnv := map[string]string{}
	for _, e := range baseCfg.Build.PassEnv {
		passedEnv[e] = os.Getenv(e)
	}

	passedSecretEnv := map[string]string{}
	for _, e := range baseCfg.Build.PassSecretEnv {
		passedSecretEnv[e] = os.Getenv(e)
	}

	baseCfg.Build.Variables = utils.MergeMaps(passedEnv, baseCfg.Build.Variables)
	baseCfg.Build.SecretVariables = utils.MergeMaps(passedSecretEnv, baseCfg.Build.SecretVariables)

	baseCfg.Build.Path = utils.StringPtr(fmt.Sprintf("%s:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin", *baseCfg.Build.Path))

	return baseCfg, nil
}

func StringPtr(str string) *string {
	return &str
}
