package config

import (
	"fmt"
	"os"
	"path/filepath"

	environs "github.com/zen-io/zen-core/environments"
	"github.com/zen-io/zen-core/utils"
	"github.com/zen-io/zen-engine/cache"
	eng_utils "github.com/zen-io/zen-engine/utils"

	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
)

type ProjectConfig struct {
	Path     string
	Zen      *ProjectZenConfig       `mapstructure:"zen"`
	Parse    *ProjectParseConfig     `mapstructure:"parse"`
	Build    *ProjectBuildConfig     `mapstructure:"build"`
	Deploy   *ProjectDeployConfig    `mapstructure:"deploy"`
	Plugins  []*ProjectPluginConfig  `mapstructure:"plugin"`
	Commands []*ProjectCommandConfig `mapstructure:"command"`
	Cache    *cache.CacheConfig      `hcl:"cache,block"`
}

type ProjectZenConfig struct {
	Version string `mapstructure:"version"`
}

type ProjectParseConfig struct {
	Filename  string   `mapstructure:"filename"`
	Placement []string `mapstructure:"placement"`
}

type ProjectBuildConfig struct {
	Toolchains      map[string]string `mapstructure:"toolchains"`
	Variables       map[string]string `mapstructure:"variables"`
	PassEnv         []string          `mapstructure:"pass_env"`
	PassSecretEnv   []string          `mapstructure:"pass_secret_env"`
	SecretVariables map[string]string // this is not passed via the file
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
	Cache  *cache.CacheManager
}

func LoadProjectConfig(configPath string, cliconfig *CliConfig) (*ProjectConfig, error) {
	if _, err := os.Stat(configPath); err != nil {
		return nil, fmt.Errorf("config %s does not exist", configPath)
	}

	projRoot := filepath.Dir(configPath)
	baseCacheRoot := filepath.Join(projRoot, ".zen")
	baseCfg := &ProjectConfig{
		Path: projRoot,
		Zen: &ProjectZenConfig{
			Version: "latest",
		},
		Parse: &ProjectParseConfig{
			Filename:  "BUILD",
			Placement: []string{},
		},
		Build: &ProjectBuildConfig{
			Toolchains: make(map[string]string),
			PassEnv:    make([]string, 0),
			Variables: map[string]string{
				"REPO_ROOT": projRoot,
			},
			PassSecretEnv: make([]string, 0),
		},
		Deploy: &ProjectDeployConfig{
			Environments: map[string]*environs.Environment{},
			Variables:    make(map[string]string),
		},
		Cache: &cache.CacheConfig{
			Tmp:      StringPtr(filepath.Join(baseCacheRoot, "cache")),
			Out:      StringPtr(filepath.Join(baseCacheRoot, "out")),
			Metadata: StringPtr(filepath.Join(baseCacheRoot, "metadata")),
			Exec:     StringPtr(filepath.Join(baseCacheRoot, "exec")),
			Artifacts:  StringPtr(filepath.Join(baseCacheRoot, "artifacts")),
			Type:     utils.StringPtr("local"),
			Config:   map[string]string{},
		},
		Plugins:  []*ProjectPluginConfig{},
		Commands: []*ProjectCommandConfig{},
	}

	unmarshalledConfig, err := eng_utils.ReadHclFile(configPath)
	if err != nil {
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

	if val, ok := unmarshalledConfig["zen"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one config block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Zen)
		}
	} else {
		loadedCfg.Zen = &ProjectZenConfig{}
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

	if val, ok := unmarshalledConfig["cache"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one Cache block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Cache)
		}
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

	if loadedCfg.Cache == nil {
		loadedCfg.Cache = &cache.CacheConfig{}
	}

	// mergo.Merge(baseCfg.Cache, loadedCfg.Cache, mergo.WithOverride)
	// mergo.Merge(baseCfg.Parse, loadedCfg.Parse, mergo.WithOverride)
	// mergo.Merge(baseCfg.Build, loadedCfg.Build, mergo.WithOverride)
	// mergo.Merge(baseCfg.Deploy, loadedCfg.Deploy, mergo.WithOverride)

	mergo.Merge(baseCfg, loadedCfg, mergo.WithOverride, mergo.WithAppendSlice)

	passedEnv := map[string]string{}
	for _, e := range append(baseCfg.Build.PassEnv, baseCfg.Build.PassSecretEnv...) {
		passedEnv[e] = os.Getenv(e)
	}
	baseCfg.Build.Variables = utils.MergeMaps(cliconfig.Build.Variables, passedEnv, baseCfg.Build.Variables, map[string]string{"PATH": *cliconfig.Build.Path})

	passedSecretEnv := map[string]string{}
	for _, e := range baseCfg.Build.PassSecretEnv {
		passedSecretEnv[e] = os.Getenv(e)
	}

	baseCfg.Build.SecretVariables = passedSecretEnv
	baseCfg.Deploy.Environments = environs.MergeEnvironmentMaps(cliconfig.Environments, baseCfg.Deploy.Environments)

	return baseCfg, nil
}

func (pc *ProjectConfig) PathForPackage(pkg string) string {
	return filepath.Join(pc.Path, pkg, pc.Parse.Filename)
}
