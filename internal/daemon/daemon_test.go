package daemon

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"codetect/internal/registry"
)

// TestDaemon_InvokesCodetectIndex_WithCorrectArgv verifies that the daemon's
// runIndex function calls `codetect index <project>` (not `codetect-index index <project>`).
//
// This is the first test for internal/daemon/. It exercises the exec.Command
// call site at daemon.go:344 by injecting a fake exec function via the
// Daemon.execFn field (added in the fix(daemon) step that follows).
//
// RED until the execFn field is added to Daemon and runIndex uses it.
func TestDaemon_InvokesCodetectIndex_WithCorrectArgv(t *testing.T) {
	var capturedName string
	var capturedArgs []string

	// Fake exec.CommandContext: records the call, returns a no-op command.
	fakeExec := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		capturedName = name
		capturedArgs = args
		// Return a real command that does nothing (true/echo) so cmd.CombinedOutput succeeds
		return exec.CommandContext(ctx, "true")
	}

	// Set up a minimal registry in a temp dir so SetLastIndexed doesn't panic.
	// Use t.TempDir() for both the registry and the fake project path so parallel
	// test runs don't race on shared filesystem state.
	tmpDir := t.TempDir()
	reg, err := registry.NewRegistryAt(filepath.Join(tmpDir, "registry.json"))
	if err != nil {
		t.Fatalf("creating test registry: %v", err)
	}
	projectPath := filepath.Join(tmpDir, "fake-project")
	if err := reg.Add(projectPath); err != nil {
		t.Fatalf("registering fake project: %v", err)
	}

	d := &Daemon{
		registry:    reg,
		indexQueue:  make(chan string, 1),
		debounceMap: make(map[string]*time.Timer),
		logger:      slog.Default(),
		execFn:      fakeExec,
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.ctx = ctx
	d.cancel = cancel

	d.runIndex(projectPath)

	if capturedName != "codetect" {
		t.Errorf("expected binary %q, got %q", "codetect", capturedName)
	}
	if len(capturedArgs) < 2 || capturedArgs[0] != "index" || capturedArgs[1] != projectPath {
		t.Errorf("expected args [\"index\", %q], got %v", projectPath, capturedArgs)
	}
}
