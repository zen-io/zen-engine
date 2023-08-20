package engine

import (
	"fmt"
	"strings"

	zen_targets "github.com/zen-io/zen-core/target"
	"github.com/zen-io/zen-core/utils"
	"github.com/zen-io/zen-engine/cache"

	out_mgr "github.com/tiagoposse/go-tasklist-out"
)

type ExecutionStep struct {
	Target        *zen_targets.Target
	Pre           func(target *zen_targets.Target, runCtx *zen_targets.RuntimeContext) error
	Post          func(target *zen_targets.Target, runCtx *zen_targets.RuntimeContext) error
	Run           func(target *zen_targets.Target, runCtx *zen_targets.RuntimeContext) error
	CheckCache    func(target *zen_targets.Target) (bool, error)
	TransformOut  func(target *zen_targets.Target, o string) (string, bool)
	Visibility    []string
	Deps          []string
	Binary        bool
	SecretEnv     map[string]string
	PassSecretEnv []string

	Local                bool
	ExternalPath         *string
	NoCacheInterpolation bool

	// This will be filled up by the engine
	Clean bool
	Cache *cache.CacheItem
	out_mgr.TaskLogger
}

func (eng *Engine) NewExecStepFromBuilder(tb *zen_targets.TargetBuilder, script string) (*ExecutionStep, error) {
	execScript, ok := tb.Scripts[script]
	if !ok {
		return nil, ScriptNotSupported{}
	}
	execScript.Env = utils.MergeMaps(eng.Projects[tb.Project()].Env, tb.Env, execScript.Env)

	if eng.Ctx.UseEnvironments && script != "build" {
		var deploy_env map[string]string
		if eng.Ctx.Env == "" {
			if len(tb.Environments) == 1 {
				for e := range tb.Environments {
					eng.Ctx.Env = e
				}
				deploy_env = tb.Environments[eng.Ctx.Env].Env()
			} else if len(tb.Environments) > 0 {
				return nil, fmt.Errorf("please provide an environment")
			} else {
				deploy_env = make(map[string]string)
			}
		} else {
			if val, ok := tb.Environments[eng.Ctx.Env]; !ok {
				return nil, fmt.Errorf("env '%s' not supported, environments supported are %s", eng.Ctx.Env, strings.Join(tb.KnownEnvironments(), ","))
			} else {
				deploy_env = val.Env()
			}
		}	

		execScript.Env = utils.MergeMaps(execScript.Env, deploy_env, map[string]string{"DEPLOY_ENV": eng.Ctx.Env})
	}

	es := &ExecutionStep{
		Target:        tb.ToTarget(script, tb.Env),
		Visibility:    tb.Visibility,
		Binary:        tb.Binary,
		Deps:          execScript.Deps,
		Pre:           execScript.Pre,
		Run:           execScript.Run,
		Post:          execScript.Post,
		TransformOut:  execScript.TransformOut,
		CheckCache:    execScript.CheckCache,
		Local:         execScript.Local,
		ExternalPath:  tb.ExternalPath,
		SecretEnv:     tb.SecretEnv,
		PassSecretEnv: tb.PassSecretEnv,
	}

	return es, nil
}
