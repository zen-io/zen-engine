package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zen-io/zen-core/target"
	zen_targets "github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-engine/cache"
	"github.com/zen-io/zen-engine/config"
	"github.com/zen-io/zen-engine/parser"

	"github.com/spf13/pflag"
	dag "github.com/tiagoposse/go-dag"
	out_mgr "github.com/tiagoposse/go-tasklist-out"
)

type Engine struct {
	cliconfig *config.CliConfig
	Projects  map[string]*config.Project
	out       *out_mgr.OutputManager
	Ctx       *target.RuntimeContext

	// DAG
	targets map[string]map[string]map[string]*target.Target
	*dag.DAG

	prePostFns map[string]*RunFnMap

	*out_mgr.TaskLoggerImpl
	*parser.PackageParser
}

func NewEngine() (*Engine, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	parser, err := parser.NewPackageParser()
	if err != nil {
		return nil, err
	}

	eng := &Engine{
		cliconfig:     cfg,
		Projects:      make(map[string]*config.Project),
		targets:       make(map[string]map[string]map[string]*target.Target),
		PackageParser: parser,
		prePostFns:    make(map[string]*RunFnMap),
	}

	return eng, nil
}

func (eng *Engine) Initialize(flags *pflag.FlagSet) (err error) {
	// Setup context
	ctx := target.NewRuntimeContext(flags, *eng.cliconfig.Build.Path, eng.cliconfig.Host.OS, eng.cliconfig.Host.Arch)
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

	// Setup the DAG
	dagOpts := []dag.Option{
		dag.WithMaxParallel(30),
	}

	if int(ui.Verbosity()) >= int(out_mgr.Debug) {
		dagOpts = append(dagOpts, dag.WithDebugFunc(func(msg string) { eng.Traceln(msg) }))
	}

	eng.DAG = dag.NewDAG(dagOpts...)

	projConfigs := make(map[string]*config.ProjectConfig)
	for projName, projPath := range eng.cliconfig.Global.Projects {
		projConfig, err := config.LoadProjectConfig(filepath.Join(projPath, ".zenconfig"), eng.cliconfig)
		if err != nil {
			return fmt.Errorf("loading project %s: %w", projName, err)
		}

		eng.Projects[projName] = &config.Project{
			Config: projConfig,
			Cache:  cache.NewCacheManager(projConfig.Cache),
		}

		eng.targets[projName] = make(map[string]map[string]*zen_targets.Target)
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
			if err := os.RemoveAll(*proj.Config.Cache.Tmp); err != nil {
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
	//  else {
	// 	return eng.NoBuildGraphAndRun(args, "clean")
	// }
	return nil
}

func (eng *Engine) RegisterCommandFunctions(fns map[string]*RunFnMap) {
	for k, v := range fns {
		eng.prePostFns[k] = v
	}
}

func (eng *Engine) ResolveTarget(fqn *target.QualifiedTargetName) ([]*target.Target, error) {
	ret := make([]*target.Target, 0)

	if eng.targets[fqn.Project()][fqn.Package()] == nil {
		eng.targets[fqn.Project()][fqn.Package()] = make(map[string]*zen_targets.Target)
		ts, err := eng.ParsePackageTargets(fqn.Project(), fqn.Package())
		if err != nil {
			return nil, fmt.Errorf("getting target %s: %w", fqn.Qn(), err)
		}

		for _, t := range ts {
			t.SetFqn(fqn.Project(), fqn.Package())
			t.SetOriginalPath(filepath.Dir(eng.Projects[fqn.Project()].Config.PathForPackage(fqn.Package())))
			t.ExpandEnvironments(eng.Projects[fqn.Project()].Config.Deploy.Environments)
			t.SetBuildVariables(eng.Projects[fqn.Project()].Config.Build.Variables)

			if err := t.EnsureValidTarget(); err != nil {
				return nil, fmt.Errorf("%s is not a valid target: %w", t.Qn(), err)
			}
			eng.targets[t.Project()][t.Package()][t.Name] = t
		}
	}

	if fqn.Name() == "all" {
		for _, t := range eng.targets[fqn.Project()][fqn.Package()] {
			ret = append(ret, t)
		}
	} else if val, ok := eng.targets[fqn.Project()][fqn.Package()][fqn.Name()]; ok {
		ret = append(ret, val)
	} else {
		return nil, fmt.Errorf("%s is not a valid step", fqn.Qn())
	}

	return ret, nil
}
