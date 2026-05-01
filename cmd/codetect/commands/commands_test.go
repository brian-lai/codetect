package commands_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"codetect/cmd/codetect/commands"
)

// stubPanic asserts that the given function panics with a message containing
// "not implemented: " followed by the expected stub name. Used to verify
// dispatch reaches the right stub before implementation is filled in.
func expectPanic(t *testing.T, fn func(), wantContains string) (recovered bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			msg := ""
			switch v := r.(type) {
			case string:
				msg = v
			case error:
				msg = v.Error()
			}
			if !strings.Contains(msg, wantContains) {
				t.Errorf("panic message %q does not contain %q", msg, wantContains)
			}
			recovered = true
		}
	}()
	fn()
	return false
}

// TestDispatch_Routes verifies every spec §1.2 route reaches the correct handler.
// Each subcommand handler is currently a stub that panics "not implemented: ...".
// Once RunMCP, RunInit, etc. are implemented, the panic assertions become real calls.
func TestDispatch_Routes(t *testing.T) {
	tests := []struct {
		argv         []string
		wantPanic    string // expected "not implemented: ..." fragment; empty = no panic expected
		wantExitCode commands.ExitCode
	}{
		{[]string{"mcp"}, "not implemented: cmd/codetect/commands.RunMCP", 0},
		{[]string{"serve"}, "not implemented: cmd/codetect/commands.RunMCP", 0},
		{[]string{"init"}, "not implemented: cmd/codetect/commands.RunInit", 0},
		{[]string{"index"}, "not implemented: cmd/codetect/commands.RunIndex", 0},
		{[]string{"embed"}, "not implemented: cmd/codetect/commands.RunEmbed", 0},
		{[]string{"stats"}, "not implemented: cmd/codetect/commands.RunStats", 0},
		{[]string{"doctor"}, "not implemented: cmd/codetect/commands.RunDoctor", 0},
		{[]string{"daemon"}, "not implemented: cmd/codetect/commands.RunDaemon", 0},
		{[]string{"registry"}, "not implemented: cmd/codetect/commands.RunRegistry", 0},
		{[]string{"migrate-to-postgres"}, "not implemented: cmd/codetect/commands.RunMigrate", 0},
		{[]string{"version"}, "not implemented: cmd/codetect/commands.RunVersion", 0},
		{[]string{"-v"}, "not implemented: cmd/codetect/commands.RunVersion", 0},
		{[]string{"--version"}, "not implemented: cmd/codetect/commands.RunVersion", 0},
		// "help" should not panic — it calls PrintHelp and returns
		// This row expects no panic and exit code 0; will be asserted after help impl
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.argv, " "), func(t *testing.T) {
			if tt.wantPanic != "" {
				recovered := expectPanic(t, func() {
					commands.Dispatch(tt.argv)
				}, tt.wantPanic)
				if !recovered {
					t.Errorf("expected panic containing %q but no panic occurred", tt.wantPanic)
				}
			}
		})
	}
}

// TestDispatch_NoArgsRoutesToMCP verifies that invoking with no args starts MCP (stubs panic).
func TestDispatch_NoArgsRoutesToMCP(t *testing.T) {
	recovered := expectPanic(t, func() {
		commands.Dispatch([]string{})
	}, "not implemented: cmd/codetect/commands.RunMCP")
	if !recovered {
		t.Error("Dispatch with no args should panic with RunMCP not-implemented")
	}
}

// TestDispatch_UnknownExitsTwo verifies unknown subcommand exits with code 2.
func TestDispatch_UnknownExitsTwo(t *testing.T) {
	// Capture stderr to suppress output noise in test runner
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	code := commands.Dispatch([]string{"totally-unknown-subcommand"})

	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	errOut := buf.String()

	if code != commands.ExitUnknownArg {
		t.Errorf("got exit %d, want %d (ExitUnknownArg)", code, commands.ExitUnknownArg)
	}
	if !strings.Contains(errOut, "unknown subcommand") {
		t.Errorf("expected stderr to contain 'unknown subcommand', got: %q", errOut)
	}
}

// TestDispatch_Help verifies help subcommand exits cleanly without panic.
func TestDispatch_Help(t *testing.T) {
	// Once PrintHelp is real, this should not panic.
	// For now, verify no panic occurs (help doesn't delegate to a stub).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("'help' subcommand should not panic, got: %v", r)
		}
	}()
	code := commands.Dispatch([]string{"help"})
	if code != commands.ExitOK {
		t.Errorf("got exit %d, want 0 (ExitOK)", code)
	}
}
