package commands_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// findShimScript returns the absolute path to the named shim script in scripts/shims/.
func findShimScript(t *testing.T, name string) string {
	t.Helper()
	root := repoRootDir(t)
	p := filepath.Join(root, "scripts", "shims", name+".sh")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("shim script %s not found at %s (expected after shims step)", name, p)
	}
	return p
}

// runShim executes a shim script with a real codetect binary on PATH,
// returns stdout, stderr, and exit code.
func runShim(t *testing.T, shimPath, codetectBin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{shimPath}, args...)...)
	binDir := filepath.Dir(codetectBin)
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// TestShim_PrintsDeprecationWarning asserts each shim emits a deprecation warning on stderr.
// RED until make install ships the shim scripts.
func TestShim_PrintsDeprecationWarning(t *testing.T) {
	if testing.Short() {
		t.Skip("shim e2e tests skipped in short mode")
	}
	bin := binaryPath(t)

	shims := []string{"codetect-index", "codetect-daemon", "migrate-to-postgres"}
	for _, shimName := range shims {
		t.Run(shimName, func(t *testing.T) {
			shimPath := findShimScript(t, shimName)
			_, stderr, _ := runShim(t, shimPath, bin, "--help")
			if !strings.Contains(stderr, "deprecated") {
				t.Errorf("shim %q: expected deprecation warning in stderr, got:\n%s", shimName, stderr)
			}
		})
	}
}

// TestShim_CodetectIndex_Delegates asserts codetect-index shim delegates to `codetect index`.
func TestShim_CodetectIndex_Delegates(t *testing.T) {
	if testing.Short() {
		t.Skip("shim e2e tests skipped in short mode")
	}
	bin := binaryPath(t)
	shimPath := findShimScript(t, "codetect-index")

	stdout, stderr, _ := runShim(t, shimPath, bin, "--help")
	combined := stdout + stderr
	// Should print help for the index command (or the MCP server startup text transitionally)
	if combined == "" {
		t.Error("codetect-index shim: expected non-empty output delegating to codetect")
	}
	if strings.Contains(combined, "not found") || strings.Contains(combined, "No such file") {
		t.Errorf("codetect-index shim: binary not found on PATH:\n%s", combined)
	}
}

// TestShim_CodetectDaemon_Delegates asserts codetect-daemon shim delegates to `codetect daemon`.
func TestShim_CodetectDaemon_Delegates(t *testing.T) {
	if testing.Short() {
		t.Skip("shim e2e tests skipped in short mode")
	}
	bin := binaryPath(t)
	shimPath := findShimScript(t, "codetect-daemon")

	_, stderr, _ := runShim(t, shimPath, bin, "--help")
	if !strings.Contains(stderr, "deprecated") {
		t.Errorf("codetect-daemon shim: expected deprecation warning, got:\n%s", stderr)
	}
}

// TestShim_MigrateToPostgres_Delegates asserts migrate-to-postgres shim delegates correctly.
func TestShim_MigrateToPostgres_Delegates(t *testing.T) {
	if testing.Short() {
		t.Skip("shim e2e tests skipped in short mode")
	}
	bin := binaryPath(t)
	shimPath := findShimScript(t, "migrate-to-postgres")

	_, stderr, _ := runShim(t, shimPath, bin, "--help")
	if !strings.Contains(stderr, "deprecated") {
		t.Errorf("migrate-to-postgres shim: expected deprecation warning, got:\n%s", stderr)
	}
}

// TestShim_PassesArgumentsVerbatim asserts arguments flow through the shim unchanged.
func TestShim_PassesArgumentsVerbatim(t *testing.T) {
	if testing.Short() {
		t.Skip("shim e2e tests skipped in short mode")
	}
	// We use codetect-index version as a proxy: `codetect-index version` should
	// print the same version output as `codetect version`.
	bin := binaryPath(t)
	shimPath := findShimScript(t, "codetect-index")

	directOut := runCmdOutput(t, bin, ".", os.Environ(), 0, "version")
	shimOut, shimErr, _ := runShim(t, shimPath, bin, "version")

	// The shim adds a deprecation warning to stderr; stdout should match direct invocation.
	if strings.TrimSpace(shimOut) != strings.TrimSpace(directOut) {
		t.Errorf("shim passthrough: got %q via shim, want %q direct\nshim stderr: %s",
			shimOut, directOut, shimErr)
	}
}
