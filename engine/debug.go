package engine

import (
	"fmt"
	"os"
	"os/exec"

	zen_targets "github.com/zen-io/zen-core/target"
)

func EnterTargetShell(target *zen_targets.Target, script string) {
	target.Scripts[script].Run = func(target *zen_targets.Target, runCtx *zen_targets.RuntimeContext) error {
		cmd := exec.Command("sh")
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
