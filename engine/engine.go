package engine

import (
	"os"

	"github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-engine/cache"
	"github.com/zen-io/zen-engine/config"
	"github.com/zen-io/zen-engine/parser"

	"github.com/spf13/pflag"
	dag "github.com/tiagoposse/go-dag"
	out_mgr "github.com/tiagoposse/go-tasklist-out"
)

type Engine struct {
	config *config.Config
	Cache  *cache.CacheManager
	out    *out_mgr.OutputManager
	Ctx    *target.RuntimeContext

	// DAG
	targets map[string]*target.Target
	*dag.DAG

	prePostFns map[string]*RunFnMap

	*out_mgr.TaskLoggerImpl
	*parser.PackageParser
}

func NewEngine() (*Engine, error) {
	config, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	parser, err := parser.NewPackageParser(config.Global.Projects)
	if err != nil {
		return nil, err
	}

	eng := &Engine{
		config:        config,
		Cache:         cache.NewCacheManager(config.Cache),
		targets:       make(map[string]*target.Target),
		PackageParser: parser,
		prePostFns:    make(map[string]*RunFnMap),
	}

	return eng, nil
}

func (eng *Engine) Initialize(flags *pflag.FlagSet) (err error) {
	// Setup context
	ctx := target.NewRuntimeContext(flags, eng.config.Environments, *eng.config.Build.Path, eng.config.Host.OS, eng.config.Host.Arch)
	eng.Ctx = ctx
	eng.PackageParser.Initialize(ctx)

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
		if err := os.RemoveAll(*eng.config.Cache.Tmp); err != nil {
			return err
		}
		if err := os.RemoveAll(*eng.config.Cache.Metadata); err != nil {
			return err
		}

		return os.RemoveAll(*eng.config.Cache.Out)
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
