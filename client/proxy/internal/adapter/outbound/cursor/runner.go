package cursor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"proxy/internal/port"
)

// Runner spawns the Cursor CLI `agent` binary as a subprocess per logical agent.
type Runner struct {
	binPath string
	workDir string

	mu    sync.Mutex
	procs map[string]*process
}

type process struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

func NewRunner(binPath, workDir string) *Runner {
	if binPath == "" {
		binPath = "agent"
	}
	if workDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			workDir = filepath.Join(home, ".samams", "workspaces")
		} else {
			workDir, _ = os.Getwd()
		}
	}
	workDir, _ = filepath.Abs(workDir)
	os.MkdirAll(workDir, 0755)
	return &Runner{
		binPath: binPath,
		workDir: workDir,
		procs:   make(map[string]*process),
	}
}

func (r *Runner) StartAgent(ctx context.Context, agentID string, opts port.StartOptions, logf port.LogFunc) (*port.Handle, error) {
	args := make([]string, 0, len(opts.CursorArgs)+5)

	// --trust: skip workspace trust prompt for non-interactive use.
	// --print (-p): script-friendly output mode (stdout streaming).
	// --yolo: auto-approve all tool calls (terminal commands, file writes).
	//         git push is blocked by pre-push hook — see EnsureGitHooks().
	args = append(args, "--trust", "--print", "--yolo")
	args = append(args, opts.CursorArgs...)

	// For long prompts (>4000 chars), write to a file in the workdir
	// and tell the agent to read it. This avoids Windows command line
	// length limit (8191 chars).
	if opts.Prompt != "" {
		if len(opts.Prompt) > 4000 {
			promptDir := opts.WorkDir
			if promptDir == "" {
				promptDir = r.workDir
			}
			promptPath := filepath.Join(promptDir, ".samams-prompt.md")
			if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0644); err == nil {
				args = append(args, "Read the file .samams-prompt.md in the current directory and follow ALL instructions in it exactly.")
			} else {
				args = append(args, opts.Prompt[:4000]+"... (truncated)")
			}
		} else {
			args = append(args, opts.Prompt)
		}
	}

	cmd := exec.CommandContext(ctx, r.binPath, args...)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	} else {
		cmd.Dir = r.workDir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	platformSetupProcess(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start agent process: %w", err)
	}

	r.mu.Lock()
	r.procs[agentID] = &process{cmd: cmd, stdin: stdin}
	r.mu.Unlock()

	go streamLines(stdout, func(line string) {
		if logf != nil {
			logf(line)
		}
	})
	go streamLines(stderr, func(line string) {
		if logf != nil {
			logf("[stderr] " + line)
		}
	})

	handle := port.NewHandle(func() error {
		defer func() {
			r.mu.Lock()
			delete(r.procs, agentID)
			r.mu.Unlock()
		}()
		return cmd.Wait()
	})

	return handle, nil
}

func (r *Runner) InterruptAgent(ctx context.Context, agentID string) error {
	r.mu.Lock()
	p, ok := r.procs[agentID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("agent %s: %w", agentID, os.ErrNotExist)
	}
	if p.cmd.Process == nil {
		return nil
	}
	// 2x signal with 0.2s interval:
	//   1st → cancel current tool call
	//   2nd → confirm exit, enter input-waiting mode (INTERRUPTED)
	for i := 0; i < 2; i++ {
		if err := sendInterruptSignal(p); err != nil {
			return err
		}
		if i < 1 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	return nil
}

func (r *Runner) StopAgent(ctx context.Context, agentID string) error {
	r.mu.Lock()
	p, ok := r.procs[agentID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("agent %s: %w", agentID, os.ErrNotExist)
	}
	if p.cmd.Process == nil {
		return nil
	}

	done := make(chan struct{})
	go func() {
		_ = p.cmd.Wait()
		close(done)
	}()

	// 4x signal (0.2s intervals) → 5s grace → Kill
	//   1st → cancel current tool call
	//   2nd → confirm exit prompt
	//   3rd → start shutdown
	//   4th → force exit
	for i := 0; i < 4; i++ {
		_ = sendInterruptSignal(p)
		select {
		case <-done:
			return nil
		case <-time.After(200 * time.Millisecond):
			continue
		}
	}
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		if err := p.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("kill agent %s after timeout: %w", agentID, err)
		}
	}
	return nil
}

func (r *Runner) SendInput(agentID, input string) error {
	r.mu.Lock()
	p, ok := r.procs[agentID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("agent %s: %w", agentID, os.ErrNotExist)
	}
	if p.stdin == nil {
		return fmt.Errorf("agent %s has no stdin", agentID)
	}
	_, err := io.WriteString(p.stdin, input+"\n")
	return err
}

func streamLines(rdr io.Reader, logf port.LogFunc) {
	if rdr == nil || logf == nil {
		return
	}
	scanner := bufio.NewScanner(rdr)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		logf(scanner.Text())
	}
}

// EnsureGitHooks installs a global pre-push hook that blocks git push.
func (r *Runner) EnsureGitHooks() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home dir: %w", err)
	}

	hooksDir := filepath.Join(home, ".samams", "git-hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-push")

	// Always overwrite — hook message may have been updated.
	// setGlobalHooksPath is called after writing regardless.

	hook := `#!/bin/sh
# SAMAMS-AGENT-GUARD: blocks git push from automated agents.
# Applies globally while agent-proxy is running.
echo "[SAMAMS] AI Agent: CursorAI is not allow to push; therefore, git push is blocked. All merges stay local. If you are AI Agent, Retrieve Immediately"
echo "[SAMAMS] For Human Only. Push manually: git -c core.hooksPath= push"
exit 1
`
	if err := os.WriteFile(hookPath, []byte(hook), 0755); err != nil {
		return fmt.Errorf("write pre-push hook: %w", err)
	}

	return r.setGlobalHooksPath(hooksDir)
}

func (r *Runner) setGlobalHooksPath(dir string) error {
	cmd := exec.Command("git", "config", "--global", "core.hooksPath", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config core.hooksPath: %s: %w", string(out), err)
	}
	return nil
}

// RemoveGitHooks restores git's default hook behavior.
func (r *Runner) RemoveGitHooks() {
	cmd := exec.Command("git", "config", "--global", "--unset", "core.hooksPath")
	_ = cmd.Run()
}

// ResolveBinary attempts to resolve the `agent` binary path.
func ResolveBinary(bin string) string {
	if bin == "" {
		bin = "agent"
	}
	if filepath.IsAbs(bin) {
		return bin
	}
	if path, err := exec.LookPath(bin); err == nil {
		return path
	}
	return bin
}
