package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"
	eng_utils "github.com/zen-io/zen-engine/utils"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type RunFnMap struct {
	Pre  func(eng *Engine, es *ExecutionStep) error
	Post func(eng *Engine, es *ExecutionStep) error
}

func (eng *Engine) _execute_step(es *ExecutionStep, resc chan<- result) {
	go func() {
		err := eng._run_step(es)
		if err != nil {
			eng.Debugln("Finished %s with error", es.Target.Fqn())
			err = fmt.Errorf("%s: %w", es.Target.Fqn(), err)
		} else {
			eng.Debugln("Finished %s", es.Target.Fqn())
			eng.out.CompleteTask(es.Target.Fqn())
		}

		resc <- result{
			name: es.Target.Fqn(),
			err:  err,
		}
	}()
}

func (eng *Engine) _run_step(es *ExecutionStep) error {
	// fqns have been verified at this point

	script := es.Target.Script()

	var err error
	if es.Target.TaskLogger, err = eng.out.CreateTask(es.Target.Fqn(), ""); err != nil {
		return err
	}
	defer es.Target.Done()


	if err := es.Target.ExpandTools(eng.Projects[es.Target.Project()].Cache.TargetOuts); err != nil {
		return fmt.Errorf("expanding tools: %w", err)
	}

	if err := es.Target.InterpolateMyself(); err != nil {
		return fmt.Errorf("interpolating myself: %w", err)
	}

	// load cache
	if script == "build" {
		es.Cache, err = eng.Projects[es.Target.Project()].Cache.LoadTargetCache(
			es.Target,
			es.ExternalPath != nil,
			filepath.Dir(eng.Projects[es.Target.Project()].Config.PathForPackage(es.Target.Package())),
		)
		if err != nil {
			return fmt.Errorf("loading cache: %w", err)
		}
	} else {
		es.Cache, _ = eng.Projects[es.Target.Project()].Cache.ToScriptCache(es.Target)
	}

	es.Target.Cwd = es.Cache.BuildCachePath()
	es.Target.Env["CWD"] = es.Target.Cwd

	projSecretEnv := eng_utils.DeepCopyStringMap(eng.Projects[es.Target.Project()].SecretEnv)

	// Secret env gets merged after cache is build, so as to not influence the cache hash
	es.Target.Env["PATH"] = strings.Join([]string{es.Target.Env["PATH"], projSecretEnv["PATH"]}, ":")
	delete(projSecretEnv, "PATH")

	for _, e := range es.PassSecretEnv {
		es.SecretEnv[e] = os.Getenv(e)
	}

	interpolEnv, err := utils.InterpolateMapWithItself(utils.MergeMaps(
		projSecretEnv,
		es.Target.Env,
		es.SecretEnv, map[string]string{"CWD": es.Target.Cwd},
	))

	if err != nil {
		return fmt.Errorf("interpolating script %s vars: %w", script, err)
	}
	es.Target.Env = interpolEnv

	// pre run
	if eng.prePostFns[script] != nil && eng.prePostFns[script].Pre != nil {
		if err := eng.prePostFns[script].Pre(eng, es); errors.Is(err, DoNotContinue{}) {
			return nil
		} else if err != nil {
			return fmt.Errorf("custom %s pre run: %w", script, err)
		}
	}

	if es.Pre != nil {
		if err := es.Pre(es.Target, eng.Ctx); errors.Is(err, DoNotContinue{}) {
			return nil
		} else if err != nil {
			return fmt.Errorf("target %s pre run: %w", script, err)
		}
	}

	if err := es.Run(es.Target, eng.Ctx); err != nil {
		es.Target.Errorln("executing run: %s", err)
		return err
	}

	// POST RUN
	// custom script post run
	if eng.prePostFns[script] != nil && eng.prePostFns[script].Post != nil {
		if err := eng.prePostFns[script].Post(eng, es); err != nil {
			return fmt.Errorf("custom %s post run: %w", script, err)
		}
	}

	// target post run
	if es.Post != nil {
		if err := es.Post(es.Target, eng.Ctx); err != nil {
			return fmt.Errorf("target %s post run: %w", script, err)
		}
	}

	if err := es.Cache.SaveMetadata(); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	eng.Debugln("Finished %s", es.Target.Fqn())

	return nil
}

func (eng *Engine) ParseArgsAndRun(flags *pflag.FlagSet, args []string, script string) {
	shell, _ := flags.GetBool("shell")
	if shell && len(args) > 1 {
		eng.Errorln("when using --shell, you can pass only one target")
		return
	}

	ts, err := eng.ExpandTargets(args, script)
	if err != nil {
		eng.Errorln("expanding target: %w", err)
		return
	}

	if err := eng.recursiveAddTargetsToGraph(ts, script); err != nil {
		eng.Errorln("building graph: %w", err)
		return
	}

	stepScripts := []string{"build"}
	if script != "build" {
		stepScripts = append(stepScripts, script)
	}

	clean, _ := flags.GetBool("clean")
	if clean {
		for _, t := range ts {
			fqn, err := target.NewFqnFromStr(t)
			if err != nil {
				eng.Errorln("inferring target fqn %s", t)
				return
			}

			for _, s := range stepScripts {
				eng.execSteps[fqn.Qn() + ":" + s].Clean = true
				eng.execSteps[fqn.Qn() + ":" + s].SecretEnv["ZEN_OPT_CLEAN"] = "true"
			}
		}
	}

	if shell {
		fqn, err := target.NewFqnFromStrWithDefault(args[0], script)
		if err != nil {
			eng.Errorln("inferring target fqn %s", args[0])
			return
		}
		EnterTargetShell(eng.execSteps[fqn.Fqn()])
	}

	// eng.PrettyPrintGraph()

	if err := eng.Run(); err != nil {
		if len(eng.errors) == 0 {
			eng.Errorln("executing the graph: %w", err)
		} else {
			for _, v := range eng.errors {
				eng.Errorln(v)
			}
		}
	}
}

func (eng *Engine) AutocompleteTargets(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	targets, err := eng.AutocompleteTarget(toComplete)
	if err != nil {
		// fmt.Println(err)
		panic(err)
	}

	return targets, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

func (eng *Engine) ExpandTargetsFromBuilder(script string) []*ExecutionStep {
	steps := []*ExecutionStep{}

	return steps
}
