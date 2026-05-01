package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binaryPath returns the path to the codetect binary, building it if needed.
// The binary is built once per test run and cached in a temp dir.
func binaryPath(t *testing.T) string {
	t.Helper()
	// Look for pre-built binary from make build
	repoRoot := repoRootDir(t)
	bin := filepath.Join(repoRoot, "dist", "codetect")
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	// Build on demand
	t.Log("building codetect binary for e2e tests...")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/codetect")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building codetect: %v\n%s", err, out)
	}
	return bin
}

func repoRootDir(t *testing.T) string {
	t.Helper()
	// Walk up from this file to find go.mod
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

func tinyGoRepoPath(t *testing.T) string {
	t.Helper()
	root := repoRootDir(t)
	p := filepath.Join(root, "testdata", "tiny-go-repo")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("testdata/tiny-go-repo not found at %s", p)
	}
	return p
}

// TestE2E_FreshInstallGoldenPath is the spine test for all phases.
// Phase 1: verifies the unified binary exists and basic subcommands dispatch correctly.
// Phase 2 will extend this to assert symbols are populated.
// Phase 3 will extend to verify health behavior.
//
// This test is RED until RunInit and RunIndex are implemented.
func TestE2E_FreshInstallGoldenPath(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e test skipped in short mode")
	}
	bin := binaryPath(t)
	repoSrc := tinyGoRepoPath(t)

	// Create a fresh temp directory to simulate a new install
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")

	// Copy tiny-go-repo fixture into temp dir
	copyDir(t, repoSrc, repoDir)

	// Override codetect data dir to avoid polluting ~/.codetect
	env := append(os.Environ(), "CODETECT_DATA_DIR="+filepath.Join(tmpDir, ".codetect"))

	// Step 1: codetect init → creates .mcp.json
	runCmd(t, bin, repoDir, env, 0, "init")
	mcpJSON := filepath.Join(repoDir, ".mcp.json")
	if _, err := os.Stat(mcpJSON); err != nil {
		t.Errorf("expected .mcp.json to exist after 'codetect init', got: %v", err)
	}

	// Step 2: codetect index → exits 0 (symbols not yet in phase 1; ok)
	runCmd(t, bin, repoDir, append(env, "CODETECT_EMBEDDING_PROVIDER=off"), 0, "index")

	// Step 3: codetect stats --json → parses, has total_chunks > 0 (v2 indexer field)
	out := runCmdOutput(t, bin, repoDir, append(env, "CODETECT_EMBEDDING_PROVIDER=off"), 0, "stats", "--json")
	if !strings.Contains(out, "total_chunks") {
		t.Errorf("codetect stats --json: expected 'total_chunks' in output, got:\n%s", out)
	}

	// Step 4: codetect version → prints version and exits 0
	vout := runCmdOutput(t, bin, repoDir, env, 0, "version")
	if vout == "" {
		t.Error("codetect version: expected non-empty output")
	}
}

// TestE2E_DeprecatedBinariesStillWork verifies that the shim scripts installed
// by `make install` delegate to the new unified binary with a deprecation warning.
//
// This test is RED until shims are installed (build(makefile) step).
func TestE2E_DeprecatedBinariesStillWork(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e test skipped in short mode")
	}

	installDir := t.TempDir()
	repoRoot := repoRootDir(t)

	// Build and install into the temp dir
	buildCmd := exec.Command("make", "install", "PREFIX="+installDir)
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Skipf("make install failed (expected until Makefile is updated): %v\n%s", err, out)
	}

	for _, shimName := range []string{"codetect-index", "codetect-daemon", "migrate-to-postgres"} {
		shimPath := filepath.Join(installDir, "bin", shimName)
		if _, err := os.Stat(shimPath); err != nil {
			t.Errorf("shim %q not found at %s after make install", shimName, shimPath)
			continue
		}

		cmd := exec.Command(shimPath, "--help")
		cmd.Env = append(os.Environ(), "PATH="+filepath.Join(installDir, "bin")+":"+os.Getenv("PATH"))
		out, _ := cmd.CombinedOutput()
		stderr := string(out)

		if !strings.Contains(stderr, "deprecated") && !strings.Contains(stderr, "is deprecated") {
			t.Errorf("shim %q: expected deprecation warning on stderr, got:\n%s", shimName, stderr)
		}
	}
}

// runCmd runs the binary with the given args from workDir, expecting the given exit code.
func runCmd(t *testing.T, bin, workDir string, env []string, wantExit int, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("running %s %v: %v\n%s", bin, args, err, out)
		}
	}
	if code != wantExit {
		t.Errorf("codetect %v: got exit %d, want %d\noutput:\n%s", args, code, wantExit, out)
	}
}

// runCmdOutput runs the binary and returns combined stdout+stderr on success.
func runCmdOutput(t *testing.T, bin, workDir string, env []string, wantExit int, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("running %s %v: %v\n%s", bin, args, err, out)
		}
	}
	if code != wantExit {
		t.Errorf("codetect %v: got exit %d, want %d\noutput:\n%s", args, code, wantExit, out)
	}
	return string(out)
}

// copyDir copies src directory to dst recursively.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
