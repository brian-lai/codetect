package commands_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"codetect/cmd/codetect/commands"
)

func TestRunVersion_PrintsVersionAndExits0(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := commands.RunVersion(nil)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if code != commands.ExitOK {
		t.Errorf("RunVersion: got exit %d, want 0", code)
	}
	if !strings.Contains(out, "codetect") {
		t.Errorf("RunVersion: output %q does not contain 'codetect'", out)
	}
	if !strings.Contains(out, commands.Version) {
		t.Errorf("RunVersion: output %q does not contain version %q", out, commands.Version)
	}
}
