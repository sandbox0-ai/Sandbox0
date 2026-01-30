package process

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// DirectRunner manages non-PTY process execution with stdout/stderr pipes.
type DirectRunner struct {
	base         *BaseProcess
	cmd          *exec.Cmd
	ctx          context.Context
	stdout       bytes.Buffer
	stderr       bytes.Buffer
	onStop       func()
	exitResolver func(error) (int, bool)
}

// NewDirectRunner creates a direct runner for a process.
func NewDirectRunner(base *BaseProcess, ctx context.Context, onStop func()) *DirectRunner {
	return &DirectRunner{
		base:   base,
		ctx:    ctx,
		onStop: onStop,
	}
}

// SetExitResolver sets a custom exit code resolver for this runner.
func (r *DirectRunner) SetExitResolver(resolver func(error) (int, bool)) {
	r.exitResolver = resolver
}

// Start launches the command with stdout/stderr pipes.
func (r *DirectRunner) Start(cmd *exec.Cmd) error {
	if r.base.IsRunning() {
		return ErrProcessAlreadyRunning
	}

	r.base.SetState(ProcessStateStarting)

	// Create a new process group for signal management
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Set up pipes for output capture
	cmd.Stdout = &r.stdout
	cmd.Stderr = &r.stderr

	if err := cmd.Start(); err != nil {
		r.base.SetState(ProcessStateCrashed)
		return fmt.Errorf("%w: %v", ErrProcessStartFailed, err)
	}

	r.cmd = cmd
	r.base.SetPID(cmd.Process.Pid)
	r.base.SetStartTime(time.Now())
	r.base.SetState(ProcessStateRunning)
	r.base.NotifyStart(StartEvent{
		ProcessID:   r.base.ID(),
		ProcessType: r.base.Type(),
		PID:         r.base.PID(),
		StartTime:   r.base.StartTime(),
		State:       r.base.State(),
		Config:      r.base.GetConfig(),
	})

	go r.monitorProcess()

	return nil
}

// Stop terminates the direct process.
func (r *DirectRunner) Stop() error {
	if !r.base.IsRunning() {
		return nil
	}

	if r.onStop != nil {
		r.onStop()
	}

	if r.cmd != nil && r.cmd.Process != nil {
		if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			_ = r.cmd.Process.Kill()
		}
	}

	r.base.SetState(ProcessStateStopped)
	r.base.CloseOutput()

	return nil
}

// GetOutput returns the captured stdout and stderr.
func (r *DirectRunner) GetOutput() (stdout, stderr string) {
	return r.stdout.String(), r.stderr.String()
}

func (r *DirectRunner) monitorProcess() {
	if r.cmd == nil {
		return
	}

	err := r.cmd.Wait()

	exitCode := 0
	if err != nil {
		if r.exitResolver != nil {
			if code, ok := r.exitResolver(err); ok {
				exitCode = code
			}
		}
		if exitCode == 0 {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if r.ctx.Err() == context.Canceled {
				exitCode = 137
			}
		}
	}

	r.base.SetExitCode(exitCode)

	duration := time.Since(r.base.StartTime())

	// Publish captured output
	if r.stdout.Len() > 0 {
		r.base.PublishOutput(ProcessOutput{
			Source: OutputSourceStdout,
			Data:   r.stdout.Bytes(),
		})
	}
	if r.stderr.Len() > 0 {
		r.base.PublishOutput(ProcessOutput{
			Source: OutputSourceStderr,
			Data:   r.stderr.Bytes(),
		})
	}

	stdoutPreview := truncatePreview(r.stdout.Bytes(), 2048)
	stderrPreview := truncatePreview(r.stderr.Bytes(), 2048)

	if exitCode == 0 {
		r.base.SetState(ProcessStateStopped)
	} else if exitCode == -1 || exitCode == 137 {
		r.base.SetState(ProcessStateKilled)
	} else {
		r.base.SetState(ProcessStateCrashed)
	}

	r.base.NotifyExit(ExitEvent{
		ProcessID:     r.base.ID(),
		ProcessType:   r.base.Type(),
		PID:           r.base.PID(),
		ExitCode:      exitCode,
		Duration:      duration,
		State:         r.base.State(),
		StdoutPreview: stdoutPreview,
		StderrPreview: stderrPreview,
		Config:        r.base.GetConfig(),
	})

	r.base.CloseOutput()
}

func truncatePreview(data []byte, limit int) string {
	if limit <= 0 || len(data) == 0 {
		return ""
	}
	if len(data) <= limit {
		return string(data)
	}
	return string(data[:limit])
}
