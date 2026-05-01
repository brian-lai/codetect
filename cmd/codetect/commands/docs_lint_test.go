package commands_test

import (
	"os/exec"
	"testing"
)

// TestDocsLint runs docs/lint_test.sh to verify that user-facing docs don't
// contain instructional uses of deprecated binary names (codetect-index, codetect-daemon).
// Allowed in deprecation notices and MIGRATION.md.
func TestDocsLint(t *testing.T) {
	root := repoRootDir(t)
	cmd := exec.Command("sh", root+"/docs/lint_test.sh")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("docs lint failed:\n%s", out)
	}
}
