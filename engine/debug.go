package engine

import (
	"fmt"
	"os"
	"os/exec"

	baulos_targets "github.com/baulos-io/baulos-core/target"
)

func EnterTargetShell(target *baulos_targets.Target, script string) {
	target.Scripts[script].Run = func(target *baulos_targets.Target, runCtx *baulos_targets.RuntimeContext) error {
		cmd := exec.Command("/bin/sh")
		// Connect the input and output of the command to the standard input and output of the Go process
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = target.GetEnvironmentVariablesList()
		cmd.Dir = target.Cwd

		// Start the command and wait for it to complete
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("starting command: %w", err)
		}
		return cmd.Wait()
	}
}
