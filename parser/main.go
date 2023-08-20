package parser

import (
	"fmt"

	"github.com/mitchellh/mapstructure"

	zen_targets "github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"
	"github.com/zen-io/zen-engine/config"

	archiving "github.com/zen-io/zen-target-archiving"
	docker "github.com/zen-io/zen-target-docker"
	exec "github.com/zen-io/zen-target-exec"
	files "github.com/zen-io/zen-target-files"
	golang "github.com/zen-io/zen-target-golang"
	k8s "github.com/zen-io/zen-target-kubernetes"
	node "github.com/zen-io/zen-target-node"
	s3 "github.com/zen-io/zen-target-s3"
	sh "github.com/zen-io/zen-target-sh"
	tf "github.com/zen-io/zen-target-terraform"
)

var PluginToTargetCreation = map[string]zen_targets.TargetCreatorMap{
	"terraform": tf.KnownTargets,
	"exec":      exec.KnownTargets,
	"files":     files.KnownTargets,
	"archiving": archiving.KnownTargets,
	"docker": docker.KnownTargets,
	"golang": golang.KnownTargets,
	"kubernetes": k8s.KnownTargets,
	"node": node.KnownTargets,
	"s3": s3.KnownTargets,
	"sh": sh.KnownTargets,
}

type PackageParser struct {
	host          *config.HostConfig
	pluginConfigs map[string]*config.ProjectPluginConfig
	parsers       map[string]zen_targets.TargetCreatorMap // types per project
	projects      map[string]*config.ProjectConfig
}

func NewPackageParser(hc *config.HostConfig) (*PackageParser, error) {
	return &PackageParser{
		host:          hc,
		pluginConfigs: make(map[string]*config.ProjectPluginConfig),
		parsers:       make(map[string]zen_targets.TargetCreatorMap),
		projects:      make(map[string]*config.ProjectConfig),
	}, nil
}

func (pp *PackageParser) Initialize(projs map[string]*config.ProjectConfig) {
	for p := range projs {
		pp.parsers[p] = make(zen_targets.TargetCreatorMap)

		for plugin, creation := range PluginToTargetCreation {
			var plugConfig *config.ProjectPluginConfig
			for _, pconf := range projs[p].Plugins {
				if pconf.Name == plugin {
					plugConfig = pconf
					break
				}
			}
			for stepType, itype := range creation {
				pp.pluginConfigs[stepType] = plugConfig
				pp.parsers[p][stepType] = itype
			}
		}
	}

	pp.projects = projs
}

func (pp *PackageParser) KnownTypes(project string) zen_targets.TargetCreatorMap {
	return pp.parsers[project]
}

func (pp *PackageParser) ParsePackageTargets(project, pkg string) ([]*zen_targets.TargetBuilder, error) {
	rr := ReadRequest{
		Vars:   pp.projects[project].Build.Env,
		Blocks: make(map[string][]map[string]interface{}),
	}

	err := rr.ReadPackageFile(pp.projects[project].PathForPackage(pkg))
	if err != nil {
		return nil, fmt.Errorf("reading package file: %w", err)
	}

	rr.Vars["CWD"] = pp.projects[project].PathForPackage(pkg)

	targets := make([]*zen_targets.TargetBuilder, 0)
	for blockType, blocks := range rr.Blocks {
		iface, ok := pp.parsers[project][blockType]
		if !ok {
			return nil, fmt.Errorf("%s is not a known task type", blockType)
		}

		for _, block := range blocks {
			ifaceBlock := iface
			if err := DecodePackage(block, &ifaceBlock); err != nil {
				return nil, err
			}

			blockTargets, err := ifaceBlock.GetTargets(&zen_targets.TargetConfigContext{
				Variables: utils.MergeMaps(
					map[string]string{"CONFIG.HOSTARCH": pp.host.Arch, "CONFIG.HOSTOS": pp.host.OS},
					pp.pluginConfigs[blockType].Env,
					pp.pluginConfigs[blockType].SecretEnv,
					rr.Vars,
				),
				KnownToolchains: pp.projects[project].Build.Toolchains,
				Environments:    pp.projects[project].Environments,
			})

			if err != nil {
				if block["name"] != nil {
					return nil, fmt.Errorf("translating block \"%s\" to targets: %w", block["name"], err)
				} else {
					return nil, fmt.Errorf("translating unnamed block to targets: %w", err)
				}
			}
			targets = append(targets, blockTargets...)
			for _, bt := range blockTargets {
				bt.SetFqn(project, pkg, bt.Name, "build")
				bt.Env = utils.MergeMaps(rr.Vars, bt.Env, pp.pluginConfigs[blockType].Env)
				bt.SecretEnv = utils.MergeMaps(pp.pluginConfigs[blockType].SecretEnv, bt.SecretEnv)
				targets = append(targets, bt)
			}
		}
	}

	return targets, nil
}

func DecodePackage(in interface{}, out interface{}) error {
	config := &mapstructure.DecoderConfig{
		Metadata:    nil,
		Result:      out,
		ErrorUnused: true,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}
	if err := decoder.Decode(in); err != nil {
		return err
	}

	return nil
}
