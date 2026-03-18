//go:build !windows

package cursor

import (
	"os"
	"os/exec"
)

// platformSetupProcess is a no-op on Unix — default process group is fine.
func platformSetupProcess(_ *exec.Cmd) {}

// sendInterruptSignal sends SIGINT to the agent process.
func sendInterruptSignal(p *process) error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Signal(os.Interrupt)
}
