package commands

import (
	"fmt"
	"os"
)

// Version is the canonical version string for the codetect binary.
// Replaces the per-binary version constants in cmd/codetect-index (3.5.3)
// and cmd/codetect (3.7.7). Phase 1 bumps to 3.8.0 to signal the binary
// collapse is complete.
const Version = "3.8.0"

// RunVersion prints the version and exits 0.
func RunVersion(args []string) ExitCode {
	fmt.Fprintf(os.Stdout, "codetect v%s\n", Version)
	return ExitOK
}
