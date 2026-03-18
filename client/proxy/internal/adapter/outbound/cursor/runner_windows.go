//go:build windows

package cursor

import (
	"io"
	"os/exec"
)

// platformSetupProcess is a no-op on Windows.
// We don't use CREATE_NEW_PROCESS_GROUP because it disables CTRL_C_EVENT.
// Instead, we send \x03 (CTRL+C) via stdin to interrupt the CLI.
func platformSetupProcess(cmd *exec.Cmd) {}

// sendInterruptSignal writes \x03 (ETX / CTRL+C) to the agent's stdin pipe.
// Cursor CLI interprets this as an interrupt: cancels current tool call
// and enters input-waiting mode (same as pressing CTRL+C in a terminal).
func sendInterruptSignal(p *process) error {
	if p.stdin == nil {
		return nil
	}
	_, err := io.WriteString(p.stdin, "\x03")
	return err
}
