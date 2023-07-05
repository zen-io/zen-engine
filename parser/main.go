package parser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zen-io/zen-engine/config"

	environs "github.com/zen-io/zen-core/environments"
	zen_targets "github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"

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
	knownTargetTypes zen_targets.TargetCreatorMap
	parsedPackages   map[string]map[string]map[string]*zen_targets.Target // project -> pkg -> name -> target
	projects         map[string]*config.Project
	contexts         map[string]*zen_targets.TargetConfigContext
}

func NewPackageParser(projs map[string]string) (*PackageParser, error) {
	projects := make(map[string]*config.Project)
	contexts := map[string]*zen_targets.TargetConfigContext{}
	parsedPackages := map[string]map[string]map[string]*zen_targets.Target{}

	// plugins := []*config.ProjectPluginConfig{}
	for projName, projPath := range projs {
		projConfig, err := config.LoadProjectConfig(fmt.Sprintf("%s/.zenconfig", projPath))
		if err != nil {
			return nil, fmt.Errorf("loading project %s: %w", projName, err)
		}

		projects[projName] = &config.Project{
			Path:   projPath,
			Config: projConfig,
		}
		parsedPackages[projName] = make(map[string]map[string]*zen_targets.Target)
	}

	return &PackageParser{
		projects:         projects,
		parsedPackages:   parsedPackages,
		knownTargetTypes: make(zen_targets.TargetCreatorMap),
		contexts:         contexts,
	}, nil
}

func (pp *PackageParser) Initialize(ctx *zen_targets.RuntimeContext) {
	for proj, projConfig := range pp.projects {
		passedEnv := map[string]string{}
		for _, e := range append(projConfig.Config.Build.PassEnv, projConfig.Config.Build.PassSecretEnv...) {
			passedEnv[e] = os.Getenv(e)
		}
		projConfig.Config.Deploy.Environments = environs.MergeEnvironmentMaps(ctx.Environments, projConfig.Config.Deploy.Environments)

		pp.contexts[proj] = &zen_targets.TargetConfigContext{
			KnownToolchains: projConfig.Config.Build.Toolchains,
			Variables:       utils.MergeMaps(ctx.Variables, projConfig.Config.Build.Variables, map[string]string{"REPO_ROOT": projConfig.Path}, passedEnv),
			Environments:    projConfig.Config.Deploy.Environments,
		}
	}

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
			pp.knownTargetTypes[stepType] = itype
		}
	}
}

// func (pp *PackageParser) LoadCommands() []*ZenCommand {
// 	for p, conf := range pp.projects {

// 	}
// }

func (pp *PackageParser) KnownTypes() zen_targets.TargetCreatorMap {
	return pp.knownTargetTypes
}

func (pp *PackageParser) ParseTargetsForBlock(project, pkg, blockType string, block interface{}, vars map[string]string) error {
	iface, ok := pp.knownTargetTypes[blockType]
	if !ok {
		return fmt.Errorf("%s is not a known task type", blockType)
	}

	if err := DecodePackage(block, &iface); err != nil {
		return err
	}

	pp.contexts[project].Variables = vars
	blockTargets, err := iface.GetTargets(pp.contexts[project])
	if err != nil {
		if block.(map[string]interface{})["name"] != nil {
			return fmt.Errorf("translating block \"%s\" to targets: %w", block.(map[string]interface{})["name"], err)
		} else {
			return fmt.Errorf("translating unnamed block to targets: %w", err)
		}
	}

	for _, target := range blockTargets {
		target.SetFqn(project, pkg)

		if err := target.EnsureValidTarget(); err != nil {
			return fmt.Errorf("//%s/%s:%s is not a valid target: %w", project, pkg, target.Name, err)
		} else {
			target.SetOriginalPath(filepath.Join(pp.projects[project].Path, pkg))

			target.ExpandEnvironments(pp.projects[project].Config.Deploy.Environments)
			target.SetBuildVariables(vars)

			pp.parsedPackages[project][pkg][target.Name] = target
		}
	}

	return nil
}

func (pp *PackageParser) ParsePackageTargets(project, pkg string) error {
	if _, ok := pp.projects[project]; !ok {
		return fmt.Errorf("project %s does not exist", project)
	}

	// check if package has already been parsed
	if _, ok := pp.parsedPackages[project][pkg]; ok {
		return nil
	}

	pp.parsedPackages[project][pkg] = map[string]*zen_targets.Target{}

	pkgFilePath := filepath.Join(pp.projects[project].Path, pkg, pp.projects[project].Config.Parse.Filename)

	// read the package
	pkgBlocks, err := pp.ReadPackageFile(pkgFilePath, project, pkg)
	if err != nil {
		return fmt.Errorf("reading package file: %w", err)
	}

	for blockType, blocks := range pkgBlocks {
		for _, block := range blocks {
			if err := pp.ParseTargetsForBlock(project, pkg, blockType, block, utils.MergeMaps(pp.contexts[project].Variables, map[string]string{"PWD": filepath.Join(pp.projects[project].Path, pkg)})); err != nil {
				var name string
				if val, ok := block["name"]; ok {
					name = val.(string)
				} else {
					name = "unknown name"
				}
				return fmt.Errorf("parsing target '%s' in pkg %s/%s: %w", name, project, pkg, err)
			}
		}
	}

	return nil
}

func (pp *PackageParser) GetTargetInPackage(fqn *zen_targets.QualifiedTargetName) (*zen_targets.Target, error) {
	if err := pp.ParsePackageTargets(fqn.Project(), fqn.Package()); err != nil {
		return nil, fmt.Errorf("getting target %s: %w", fqn.Qn(), err)
	}

	if val, ok := pp.parsedPackages[fqn.Project()][fqn.Package()][fqn.Name()]; !ok {
		return nil, fmt.Errorf("%s is not a valid step inside //%s/%s", fqn.Name(), fqn.Project(), fqn.Package())
	} else {
		return val, nil
	}
}

func (pp *PackageParser) GetAllTargetsInPackage(project, pkg string) (map[string]*zen_targets.Target, error) {
	pkgFqn := fmt.Sprintf("%s/%s", project, pkg)

	if err := pp.ParsePackageTargets(project, pkg); err != nil {
		return nil, fmt.Errorf("getting all targets in //%s: %w", pkgFqn, err)
	}

	return pp.parsedPackages[project][pkg], nil
}

func (pp *PackageParser) ProjectBuildConfig(project string) *config.ProjectBuildConfig {
	return pp.projects[project].Config.Build
}

func (pp *PackageParser) ProjectDeployConfig(project string) *config.ProjectDeployConfig {
	return pp.projects[project].Config.Deploy
}

func (pp *PackageParser) ProjectPath(project string) string {
	return pp.projects[project].Path
}
func (pp *PackageParser) ProjectFilename(project string) string {
	return pp.projects[project].Config.Parse.Filename
}

func (pp *PackageParser) IsProjectConfigured(project string) bool {
	return pp.projects[project] != nil
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
	// decoder.Decode(in)

	return nil
}
