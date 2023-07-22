package parser

import (
	"fmt"

	zen_targets "github.com/zen-io/zen-core/target"
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

	"github.com/mitchellh/mapstructure"
)

type PackageParser struct {
	parsers  map[string]zen_targets.TargetCreatorMap // types per project
	projects map[string]*config.ProjectConfig
}

func NewPackageParser() (*PackageParser, error) {

	// plugins := []*config.ProjectPluginConfig{}

	return &PackageParser{
		parsers:  make(map[string]zen_targets.TargetCreatorMap),
		projects: make(map[string]*config.ProjectConfig),
	}, nil
}

func (pp *PackageParser) Initialize(projs map[string]*config.ProjectConfig) {
	for p := range projs {
		pp.parsers[p] = make(zen_targets.TargetCreatorMap)
		for _, t := range []zen_targets.TargetCreatorMap{
			archiving.KnownTargets,
			docker.KnownTargets,
			golang.KnownTargets,
			files.KnownTargets,
			k8s.KnownTargets,
			node.KnownTargets,
			s3.KnownTargets,
			tf.KnownTargets,
			exec.KnownTargets,
			sh.KnownTargets,
		} {
			for stepType, itype := range t {
				pp.parsers[p][stepType] = itype
			}
		}
	}

	pp.projects = projs
}

func (pp *PackageParser) KnownTypes(project string) zen_targets.TargetCreatorMap {
	return pp.parsers[project]
}

func (pp *PackageParser) ParsePackageTargets(project, pkg string) ([]*zen_targets.Target, error) {
	rr := ReadRequest{
		Vars:   pp.projects[project].Build.Variables,
		Blocks: make(map[string][]map[string]interface{}),
	}

	err := rr.ReadPackageFile(pp.projects[project].PathForPackage(pkg))
	if err != nil {
		return nil, fmt.Errorf("reading package file: %w", err)
	}

	rr.Vars["PWD"] = pp.projects[project].PathForPackage(pkg)

	targets := make([]*zen_targets.Target, 0)
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
				Variables:       rr.Vars,
				KnownToolchains: pp.projects[project].Build.Toolchains,
				Environments:    pp.projects[project].Deploy.Environments,
			})

			if err != nil {
				if block["name"] != nil {
					return nil, fmt.Errorf("translating block \"%s\" to targets: %w", block["name"], err)
				} else {
					return nil, fmt.Errorf("translating unnamed block to targets: %w", err)
				}
			}

			for _, bt := range blockTargets {
				bt.SetFqn(project, pkg)
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
