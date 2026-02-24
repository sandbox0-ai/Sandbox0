package process

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPTYRunner_FastCommandOutputReliableRepeat(t *testing.T) {
	const runs = 30

	for i := 0; i < runs; i++ {
		base := NewBaseProcess("pty-fast-test", ProcessTypeCMD, ProcessConfig{
			Type: ProcessTypeCMD,
		})
		runner := NewPTYRunner(base, nil, nil)
		ch := base.ReadOutput()

		cmd := exec.Command("/bin/echo", "Hello, Sandbox0!")
		if err := runner.Start(cmd, &PTYSize{Rows: 40, Cols: 120}); err != nil {
			t.Fatalf("run %d: runner.Start() failed: %v", i, err)
		}

		output := waitPTYOutput(t, ch, 3*time.Second)
		if !strings.Contains(output, "Hello, Sandbox0!") {
			t.Fatalf("run %d: output = %q, want to contain message", i, output)
		}
	}
}

func waitPTYOutput(t *testing.T, ch <-chan ProcessOutput, timeout time.Duration) string {
	t.Helper()
	deadline := time.After(timeout)
	var b strings.Builder
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for PTY output, got=%q", b.String())
		case msg, ok := <-ch:
			if !ok {
				return b.String()
			}
			if msg.Source == OutputSourcePTY && len(msg.Data) > 0 {
				b.Write(msg.Data)
			}
		}
	}
}
