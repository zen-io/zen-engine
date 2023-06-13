package engine

import (
	"fmt"
	"strings"

	zen_targets "github.com/zen-io/zen-core/target"

	"golang.org/x/exp/slices"
)

func (eng *Engine) BuildGraph(targets []string, defaultScript string) error {
	ts, err := eng.ExpandTargets(targets, defaultScript)
	if err != nil {
		return err
	}

	if err := eng.recursiveAddTargetsToGraph(ts); err != nil {
		return err
	}

	return nil
}

// addVertex adds a function as a vertex in the graph. Only functions which have been added in this
// way will be executed eng.Run.
func (eng *Engine) recursiveAddTargetsToGraph(targets []string) error {
	if len(targets) == 0 {
		return nil
	}

	targetFqn := targets[0]
	targets = targets[1:]

	fqn, err := zen_targets.NewFqnFromStr(targetFqn)
	if err != nil {
		return err
	}

	target, err := eng.GetTargetInPackage(fqn)
	if err != nil {
		return fmt.Errorf("getting target %s: %w", targetFqn, err)
	}

	if target.Scripts[fqn.Script()] == nil {
		return eng.recursiveAddTargetsToGraph(targets)
	}

	// actually add to the graph
	eng.AddVertex(targetFqn, func() error { return eng._run_step(targetFqn) })
	if eng.targets[target.Qn()] == nil {
		eng.targets[target.Qn()] = target
	}

	depFqns, err := eng.getDependenciesToAdd(fqn.Script(), target, targets)
	if err != nil {
		return err
	}

	for _, dFqn := range depFqns {
		targets = append(targets, dFqn)
		eng.AddEdge(dFqn, targetFqn)
	}

	if fqn.Script() != "build" {
		targets = append(targets, fqn.BuildFqn())
		eng.AddEdge(fqn.BuildFqn(), targetFqn)
	}

	return eng.recursiveAddTargetsToGraph(targets)
}

func (eng *Engine) getDependenciesToAdd(script string, target *zen_targets.Target, leftoverTargets []string) ([]string, error) {
	depsToCheck := []string{}
	for _, depFqn := range target.Scripts[script].Deps {
		fqn, err := zen_targets.NewFqnFromStr(depFqn)
		if err != nil {
			return nil, err
		}

		if fqn.Name() == "all" {
			depTargets, err := eng.GetAllTargetsInPackage(fqn.Project(), fqn.Package())
			if err != nil {
				return nil, fmt.Errorf("getting all targets for %s: %w", depFqn, err)
			}

			for _, t := range depTargets {
				if t.Scripts[script] != nil {
					depsToCheck = append(depsToCheck, fmt.Sprintf("%s:%s", t.Qn(), fqn.Script()))
				}
			}
		} else {
			depsToCheck = append(depsToCheck, depFqn)
		}
	}
	pkgFqn := fmt.Sprintf("//%s/%s", target.Project(), target.Package())

	finalDeps := []string{}
	for _, d := range depsToCheck {
		if script == "build" || strings.HasPrefix(d, pkgFqn) {
			finalDeps = append(finalDeps, d)
		}

		if eng.HasVertex(d) || slices.Contains(leftoverTargets, d) {
			finalDeps = append(finalDeps, d)
		}
	}

	return finalDeps, nil
}
