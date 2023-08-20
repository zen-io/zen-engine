package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	environs "github.com/zen-io/zen-core/environments"
	"github.com/zen-io/zen-core/target"
	zen_targets "github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"
	"github.com/zen-io/zen-engine/cache"
	"github.com/zen-io/zen-engine/config"
	"github.com/zen-io/zen-engine/parser"

	"github.com/spf13/pflag"
	out_mgr "github.com/tiagoposse/go-tasklist-out"
)

type Engine struct {
	cliconfig *config.CliConfig
	Projects  map[string]*config.Project
	out       *out_mgr.OutputManager
	Ctx       *zen_targets.RuntimeContext

	// DAG
	maxParallel int
	builders    map[string]map[string]map[string]*zen_targets.TargetBuilder // project => pkg => target name target builder
	execSteps   map[string]*ExecutionStep                                   // project => pkg => target name => script step
	graph       map[string][]string
	errors      map[string]string

	prePostFns map[string]*RunFnMap

	*out_mgr.TaskLoggerImpl
	*parser.PackageParser
}

func NewEngine() (*Engine, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	parser, err := parser.NewPackageParser(cfg.Host)
	if err != nil {
		return nil, err
	}

	eng := &Engine{
		cliconfig:     cfg,
		Projects:      make(map[string]*config.Project),
		execSteps:     make(map[string]*ExecutionStep),
		builders:      make(map[string]map[string]map[string]*zen_targets.TargetBuilder),
		graph:         make(map[string][]string),
		errors:        make(map[string]string),
		PackageParser: parser,
		prePostFns:    make(map[string]*RunFnMap),
		maxParallel:   20,
	}

	return eng, nil
}

func (eng *Engine) Initialize(flags *pflag.FlagSet, ctx *target.RuntimeContext) (err error) {
	// Setup context
	eng.Ctx = ctx

	// Setup UI
	uiOpts := []out_mgr.OutputManagerOption{
		out_mgr.WithOut(os.Stdout),
	}

	if verbosity, _ := flags.GetInt("verbosity"); verbosity != 0 {
		uiOpts = append(uiOpts, out_mgr.WithVerbosity(out_mgr.VerbosityLevel(verbosity)))
	}

	if raw, _ := flags.GetBool("raw-output"); raw {
		uiOpts = append(uiOpts, out_mgr.WithRawOutput())
	}
	if keep, _ := flags.GetBool("keep-output"); keep {
		uiOpts = append(uiOpts, out_mgr.WithKeepOutput())
	}

	if shell, _ := flags.GetBool("shell"); shell {
		uiOpts = append(uiOpts, out_mgr.WithRawOutput())
	}

	// output.WithLogsRoot(*config.Cache.Exec),
	ui, err := out_mgr.NewOutputManager(uiOpts...)
	if err != nil {
		return err
	}
	ui.Start()

	eng.out = ui

	eng.TaskLoggerImpl, err = ui.CreateTask("engine", "", out_mgr.WithHidden())
	if err != nil {
		return err
	}

	baseEnv := eng.cliconfig.Build.Env
	baseSecretEnv := eng.cliconfig.Build.SecretEnv
	baseSecretEnv["PATH"] = strings.Join([]string{*eng.cliconfig.Build.Path, "/usr/local/bin:/usr/bin:/bin", "/usr/sbin:/sbin"}, ":")
	for _, e := range eng.cliconfig.Build.PassEnv {
		baseEnv[e] = os.Getenv(e)
	}

	for _, e := range eng.cliconfig.Build.PassSecretEnv {
		baseSecretEnv[e] = os.Getenv(e)
	}

	projConfigs := make(map[string]*config.ProjectConfig)
	for projName, projPath := range eng.cliconfig.Global.Projects {
		projConfig, err := config.LoadProjectConfig(filepath.Join(projPath, ".zenconfig"))
		if err != nil {
			return fmt.Errorf("loading project %s: %w", projName, err)
		}

		eng.Projects[projName] = &config.Project{
			Config:    projConfig,
			Cache:     cache.NewCacheManager(projConfig.Cache),
			Env:       utils.MergeMaps(projConfig.Build.Env, baseEnv),
			SecretEnv: make(map[string]string),
		}

		for _, e := range projConfig.Build.PassEnv {
			eng.Projects[projName].Env[e] = os.Getenv(e)
		}
		for _, e := range projConfig.Build.SecretEnv {
			eng.Projects[projName].SecretEnv[e] = os.Getenv(e)
		}

		eng.Projects[projName].SecretEnv = utils.MergeMaps(baseSecretEnv, eng.Projects[projName].SecretEnv)

		projConfig.Environments = environs.MergeEnvironmentMaps(eng.cliconfig.Environments, projConfig.Environments)

		eng.builders[projName] = make(map[string]map[string]*zen_targets.TargetBuilder)
		projConfigs[projName] = projConfig
	}

	eng.PackageParser.Initialize(projConfigs)
	return nil
}

func (e *Engine) Done() {
	// In cases where the error comes from Cobra itself, the engine might not be initialized yet
	if e == nil {
		return
	}

	e.TaskLoggerImpl.Done()
	e.out.Stop()

	e.Debug("finished")
}

func (eng *Engine) CleanCache(args []string) error {
	if len(args) == 0 {
		for _, proj := range eng.Projects {
			if err := os.RemoveAll(*proj.Config.Cache.Gen); err != nil {
				return err
			}
			if err := os.RemoveAll(*proj.Config.Cache.Metadata); err != nil {
				return err
			}
			if err := os.RemoveAll(*proj.Config.Cache.Out); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

func (eng *Engine) RegisterCommandFunctions(fns map[string]*RunFnMap) {
	for k, v := range fns {
		eng.prePostFns[k] = v
	}
}

func (eng *Engine) ResolveExecutionSteps(fqn *target.QualifiedTargetName) ([]*ExecutionStep, error) {
	if eng.builders[fqn.Project()][fqn.Package()] == nil {
		eng.builders[fqn.Project()][fqn.Package()] = make(map[string]*zen_targets.TargetBuilder)
		tbs, err := eng.ParsePackageTargets(fqn.Project(), fqn.Package())
		if err != nil {
			return nil, fmt.Errorf("parsing target %s: %w", fqn.Qn(), err)
		}

		for _, tb := range tbs {
			tb.SetOriginalPath(filepath.Dir(eng.Projects[fqn.Project()].Config.PathForPackage(fqn.Package())))
			tb.ExpandEnvironments(eng.Projects[fqn.Project()].Config.Environments)
			
			
			if err := tb.EnsureValidTarget(); err != nil {
				return nil, fmt.Errorf("%s is not a valid target: %w", tb.Qn(), err)
			}

			eng.builders[tb.Project()][tb.Package()][tb.Name] = tb
		}
	}

	stepScripts := []string{"build"}
	if fqn.Script() != "build" {
		stepScripts = append(stepScripts, fqn.Script())
	}
	stepNames := make([]string, 0)

	if fqn.Name() == "all" {
		for name := range eng.builders[fqn.Project()][fqn.Package()] {
			stepNames = append(stepNames, name)
		}
	} else if _, ok := eng.builders[fqn.Project()][fqn.Package()][fqn.Name()]; ok {
		stepNames = append(stepNames, fqn.Name())
	} else {
		return nil, fmt.Errorf("%s is not a valid step", fqn.Qn())
	}

	ret := make([]*ExecutionStep, 0)

	for _, name := range stepNames {
		builder := eng.builders[fqn.Project()][fqn.Package()][name]

		for _, s := range stepScripts {
			if val, err := eng.NewExecStepFromBuilder(builder, s); err == nil {
				ret = append(ret, val)
			} else if err != nil && !errors.Is(ScriptNotSupported{}, err) {
				return nil, err
			}
		}
	}

	return ret, nil
}
