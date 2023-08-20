package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	environs "github.com/zen-io/zen-core/environments"
	"github.com/zen-io/zen-core/utils"
	"github.com/zen-io/zen-engine/cache"
	eng_utils "github.com/zen-io/zen-engine/utils"

	"dario.cat/mergo"
	gplugin "github.com/hashicorp/go-plugin"
	"github.com/mitchellh/mapstructure"
)

type ProjectConfig struct {
	Path         string
	Zen          *ProjectZenConfig                `mapstructure:"zen"`
	Parse        *ProjectParseConfig              `mapstructure:"parse"`
	Build        *ProjectBuildConfig              `mapstructure:"build"`
	Environments map[string]*environs.Environment `mapstructure:"environments"`
	Plugins      []*ProjectPluginConfig           `mapstructure:"plugin"`
	Commands     []*ProjectCommandConfig          `mapstructure:"command"`
	Cache        *cache.CacheConfig               `hcl:"cache,block"`
}

type ProjectZenConfig struct {
	Version string `mapstructure:"version"`
}

type ProjectParseConfig struct {
	Filename  string   `mapstructure:"filename"`
	Placement []string `mapstructure:"placement"`
}

type ProjectBuildConfig struct {
	Toolchains map[string]string `mapstructure:"toolchains"`
	Env        map[string]string `mapstructure:"env"`
	PassEnv    []string          `mapstructure:"pass_env"`
	SecretEnv  []string          `mapstructure:"secret_env"`
}

type ProjectPluginConfig struct {
	Name          string            `mapstructure:"name"`
	Repo          *string           `mapstructure:"repo"`
	Path          *string           `mapstructure:"path"`
	DevCmd        *string           `mapstructure:"dev_cmd"`
	Env           map[string]string `mapstructure:"env"`
	PassEnv       []string          `mapstructure:"pass_env"`
	PassSecretEnv []string          `mapstructure:"pass_secret_env"`
	SecretEnv     map[string]string `mapstructure:"secret_env"`
	Client        *gplugin.Client
}

func (ppc *ProjectPluginConfig) ExecCmd() []string {
	var cmdArgs []string
	if ppc.DevCmd != nil {
		cmdArgs = strings.Split(*ppc.DevCmd, " ")
	} else {
		cmdArgs = []string{*ppc.Path}
	}

	return cmdArgs
}

func (ppc *ProjectPluginConfig) ExpandPluginAcceptedModules() ([]string, error) {
	var cmdArgs []string
	if ppc.DevCmd != nil {
		cmdArgs = strings.Split(*ppc.DevCmd, " ")
	} else {
		cmdArgs = []string{*ppc.Path}
	}

	cmdArgs = append(cmdArgs, "accepted")
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("executing the call: %v", err)
	}

	// Parse the returned JSON
	var data []string
	err = json.Unmarshal(out.Bytes(), &data)
	if err != nil {
		return nil, fmt.Errorf("parsing return: %v", err)
	}

	return data, nil
}

type ProjectCommandConfig struct {
	Name            string  `mapstructure:"name"`
	Repo            *string `mapstructure:"repo"`
	Path            *string `mapstructure:"path"`
	UseEnvironments bool    `mapstructure:"use_environments"`
}

type Project struct {
	Path   string
	Config *ProjectConfig
	Cache  *cache.CacheManager

	Env       map[string]string
	SecretEnv map[string]string
}

func LoadProjectConfig(configPath string) (*ProjectConfig, error) {
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
			Env: map[string]string{
				"REPO_ROOT": projRoot,
			},
			SecretEnv: make([]string, 0),
		},
		Environments: map[string]*environs.Environment{},
		Cache: &cache.CacheConfig{
			Gen:       StringPtr(filepath.Join(baseCacheRoot, "cache")),
			Out:       StringPtr(filepath.Join(baseCacheRoot, "out")),
			Metadata:  StringPtr(filepath.Join(baseCacheRoot, "metadata")),
			Logs:      StringPtr(filepath.Join(baseCacheRoot, "logs")),
			Artifacts: StringPtr(filepath.Join(baseCacheRoot, "artifacts")),
			Type:      utils.StringPtr("local"),
			Config:    map[string]string{},
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

	if val, ok := unmarshalledConfig["environments"]; ok {
		if len(val) > 1 {
			return nil, fmt.Errorf("only one environments block allowed")
		} else {
			mapstructure.Decode(val[0], &loadedCfg.Environments)
		}
	} else {
		loadedCfg.Environments = make(map[string]*environs.Environment)
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
		err := mapstructure.Decode(val, &loadedCfg.Commands)
		if err != nil {
			panic(err)
		}
	} else {
		loadedCfg.Commands = []*ProjectCommandConfig{}
	}

	if loadedCfg.Cache == nil {
		loadedCfg.Cache = &cache.CacheConfig{}
	}

	mergo.Merge(baseCfg, loadedCfg, mergo.WithOverride, mergo.WithAppendSlice)
	for _, p := range baseCfg.Plugins {
		if p.SecretEnv == nil {
			p.SecretEnv = make(map[string]string)
		}
		if p.Env == nil {
			p.Env = make(map[string]string)
		}
		if p.PassSecretEnv != nil {
			for _, e := range p.PassSecretEnv {
				p.SecretEnv[e] = os.Getenv(e)
			}
		}

		if p.PassEnv != nil {
			for _, e := range p.PassEnv {
				p.Env[e] = os.Getenv(e)
			}
		}
	}

	return baseCfg, nil
}

func (pc *ProjectConfig) PathForPackage(pkg string) string {
	return filepath.Join(pc.Path, pkg, pc.Parse.Filename)
}
