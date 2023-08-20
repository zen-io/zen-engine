package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	zen_targets "github.com/zen-io/zen-core/target"

	"golang.org/x/exp/slices"
)

var errMissingVertex = errors.New("missing vertex")
var errCycleDetected = errors.New("dependency cycle detected")
var errGraphExecError = errors.New("traversing the graph")

type result struct {
	name string
	err  error
}

func (eng *Engine) HasVertex(vertex string) bool {
	_, ok := eng.execSteps[vertex]
	return ok
}

func (eng *Engine) AddVertex(es *ExecutionStep) bool {
	if _, ok := eng.execSteps[es.Target.Fqn()]; !ok {
		eng.execSteps[es.Target.Fqn()] = es
		if eng.graph[es.Target.Fqn()] == nil {
			eng.graph[es.Target.Fqn()] = make([]string, 0)
		}
		return true
	}

	return false
}

// AddEdge establishes a dag.pendency between two vertices in the graph. Both from and to must exist
// in the graph, or Run will err. The vertex at from will execute before the vertex at to.
func (eng *Engine) AddEdge(from, to string) {
	if !slices.Contains(eng.graph[from], to) {
		eng.Traceln("Add edge from %s to %s", from, to)
		eng.graph[from] = append(eng.graph[from], to)
	}
}

func (eng *Engine) detectCycles() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for vertex := range eng.graph {
		if !visited[vertex] {
			if eng.detectCyclesHelper(vertex, visited, recStack) {
				return true
			}
		}
	}
	return false
}

func (eng *Engine) detectCyclesHelper(vertex string, visited, recStack map[string]bool) bool {
	visited[vertex] = true
	recStack[vertex] = true

	for _, v := range eng.graph[vertex] {
		// only check cycles on a vertex one time
		if !visited[v] {
			if eng.detectCyclesHelper(v, visited, recStack) {
				return true
			}
			// if we've visited this vertex in this recursion stack, then we have a cycle
		} else if recStack[v] {
			return true
		}
	}
	recStack[vertex] = false
	return false
}

// PrettyPrintGraph will print the graph
func (eng *Engine) PrettyPrintGraph() error {
	if b, err := json.MarshalIndent(&eng.graph, "", "  "); err != nil {
		return err
	} else {
		fmt.Println(string(b))
	}

	return nil
}

// Run will validate that all edges in the graph point to existing vertices, and that there are
// no dag.pendency cycles. After validation, each vertex will be run, dag.terministically, in parallel
// topological order. If any vertex returns an error, no more vertices will be scheduled and
// Run will exit and return that error once all in-flight functions finish execution.
func (eng *Engine) Run() error {
	// sanity check
	if len(eng.graph) == 0 {
		return nil
	}

	// count how many deps each vertex has
	deps := make(map[string]int)
	for vertex, edges := range eng.graph {
		// every vertex along every edge must have an associated fn
		if _, ok := eng.graph[vertex]; !ok {
			return fmt.Errorf("%s: %s", errMissingVertex, vertex)
		}

		for _, edgeVertex := range edges {
			if _, ok := eng.graph[edgeVertex]; !ok {
				return fmt.Errorf("missing edge for %s: %s", edgeVertex, vertex)
			}

			deps[edgeVertex]++
		}
	}

	if eng.detectCycles() {
		return errCycleDetected
	}

	running := 0
	done := 0
	resc := make(chan result, len(eng.graph))

	runQueue := []string{}
	// add any vertex that has no deps to the run queue
	for name := range eng.graph {
		if deps[name] == 0 {
			runQueue = append(runQueue, name)
		}
	}

	for done < len(eng.graph) {
		newQueue := []string{}
		for _, vertex := range runQueue {
			if eng.maxParallel == 0 || running < eng.maxParallel {
				running++
				eng._execute_step(eng.execSteps[vertex], resc)
			} else {
				newQueue = append(newQueue, vertex)
			}
		}

		res := <-resc
		running--
		done++

		// don't enqueue any more work on if there's been an error
		if res.err != nil {
			eng.errors[res.name] = res.err.Error()
			break
		}

		// start any vertex whose deps are fully resolved
		for _, vertex := range eng.graph[res.name] {
			if deps[vertex]--; deps[vertex] == 0 {
				newQueue = append(newQueue, vertex)
			}
		}

		runQueue = newQueue
	}

	for running > 0 {
		<-resc
		running--
	}

	if len(eng.errors) > 0 {
		return errGraphExecError
	}

	return nil
}

func (eng *Engine) recursiveAddTargetsToGraph(targets []string, script string) error {
	if len(targets) == 0 {
		return nil
	}
	
	targetFqn := targets[0]
	targets = targets[1:]

	if eng.HasVertex(targetFqn) {
		return eng.recursiveAddTargetsToGraph(targets, script)
	}

	fqn, err := zen_targets.NewFqnFromStr(targetFqn)
	if err != nil {
		return fmt.Errorf("calculating fqn from %s: %w", fqn, err)
	}

	steps, err := eng.ResolveExecutionSteps(fqn)
	if err != nil {
		return fmt.Errorf("resolving target %s: %w", targetFqn, err)
	}

	if len(steps) == 0 {
		return eng.recursiveAddTargetsToGraph(targets, script)
	}

	// actually add to the graph
	for _, step := range steps {
		step := step
		eng.AddVertex(step)
		depFqns, err := eng.getDependenciesToAdd(step.Target.Script(), step, targets)
		if err != nil {
			return fmt.Errorf("retrieving dependencies from %s: %w", step.Target.Fqn(), err)
		}

		for _, dFqn := range depFqns {
			targets = append(targets, dFqn)
			eng.AddEdge(dFqn, step.Target.Fqn())
		}
	}

	return eng.recursiveAddTargetsToGraph(targets, script)
}

func (eng *Engine) getDependenciesToAdd(script string, es *ExecutionStep, leftoverTargets []string) ([]string, error) {
	depsToCheck := []string{}

	for _, depFqn := range es.Deps {
		fqn, err := zen_targets.NewFqnFromStrWithDefault(depFqn, "build")
		if err != nil {
			return nil, fmt.Errorf("calculating fqn from str %s: %w", depFqn, err)
		}

		depExecSteps, err := eng.ResolveExecutionSteps(fqn)
		if err != nil {
			return nil, fmt.Errorf("getting all targets for %s: %w", depFqn, err)
		}
		for _, t := range depExecSteps {
			depsToCheck = append(depsToCheck, fmt.Sprintf("%s:%s", t.Target.Qn(), fqn.Script()))
		}
	}

	pkgFqn := fmt.Sprintf("//%s/%s", es.Target.Project(), es.Target.Package())

	finalDeps := []string{}
	for _, d := range depsToCheck {
		/*
		 	a dep will be added if the ref target:
			- is already in the graph
			- is to be added to the graph
			- is in the same package of the original target
			- this is a build script
			- --without-deps was not passed
		*/
		if script == "build" || strings.HasPrefix(d, pkgFqn) || eng.HasVertex(d) || slices.Contains(leftoverTargets, d) || eng.Ctx.WithDeps {
			finalDeps = append(finalDeps, d)
		}
	}

	return finalDeps, nil
}
