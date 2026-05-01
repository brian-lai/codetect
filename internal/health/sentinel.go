// Package health manages the embedding-health sentinel file that lets the
// CLI, daemon, and doctor command share a single source of truth about
// whether a project's index is usable.
//
// STUB — created by /para:plan for 2026-05-01-codetect-tier1-unbreak.
// Implementation is provided by plan phase 3. Sentinel file contract is
// defined in context/data/2026-05-01-codetect-tier1-unbreak-spec.md §3.4.
package health

import (
	"io"
	"time"
)

// Severity reflects how degraded a project's index is. See spec §3.2.
type Severity string

const (
	SeverityDegraded Severity = "degraded" // 0 < health_ratio < 0.80
	SeverityFailed   Severity = "failed"   // ChunksEmbedded == 0 && ChunksCreated > 0
)

// ProjectStatus is one entry in the sentinel file's per-project map.
type ProjectStatus struct {
	Severity       Severity  `json:"severity"`
	LastRunAt      time.Time `json:"last_run_at"`
	ChunksCreated  int       `json:"chunks_created"`
	ChunksEmbedded int       `json:"chunks_embedded"`
	ChunksFailed   int       `json:"chunks_failed"`
	SampleError    string    `json:"sample_error,omitempty"`
	Model          string    `json:"model,omitempty"`
}

// Sentinel is the top-level schema of the sentinel file. See spec §3.4.
type Sentinel struct {
	SchemaVersion int                       `json:"schema_version"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Projects      map[string]ProjectStatus  `json:"projects"`
}

// HealthyThreshold is the minimum ratio of embedded:created chunks that
// counts as a healthy run. See spec §3.1.
const HealthyThreshold = 0.80

// Store reads and writes the sentinel file at a configurable path.
type Store struct {
	path string
}

// NewStore opens (or prepares to create) the sentinel file at the given path.
// If path is empty, uses DefaultPath(). Creates parent directory if missing.
func NewStore(path string) (*Store, error) {
	panic("not implemented: internal/health.NewStore")
}

// DefaultPath returns the default sentinel path, honoring XDG_CONFIG_HOME.
// Example: "/Users/blai/.config/codetect/unhealthy.json".
func DefaultPath() string {
	panic("not implemented: internal/health.DefaultPath")
}

// Load reads the current sentinel file. Returns an empty Sentinel (not an
// error) if the file does not exist. Returns an error only on I/O or parse
// failure.
func (s *Store) Load() (*Sentinel, error) {
	panic("not implemented: internal/health.Store.Load")
}

// Upsert sets (or replaces) the status for a single project and persists.
// If after the upsert the projects map is empty, the file is deleted.
func (s *Store) Upsert(repoRoot string, status ProjectStatus) error {
	panic("not implemented: internal/health.Store.Upsert")
}

// Remove deletes the entry for a single project. If the file becomes empty,
// the file itself is deleted. No-op if the project is not present.
func (s *Store) Remove(repoRoot string) error {
	panic("not implemented: internal/health.Store.Remove")
}

// CheckResult is what EvaluateRun returns.
type CheckResult struct {
	Healthy      bool
	Severity     Severity // set only when !Healthy
	HealthRatio  float64  // ChunksEmbedded / max(ChunksCreated, 1)
	ShouldBanner bool     // print the stderr banner (spec §3.3)
	ExitCode     int      // 0 / 2 (spec §3.2)
}

// IndexRun summarizes what we need from an index/embed run to evaluate health.
// Filled in by callers in cmd/codetect/commands/index.go and embed.go.
type IndexRun struct {
	RepoRoot       string
	ChunksCreated  int
	ChunksEmbedded int
	ChunksFailed   int
	SampleError    string // first non-empty error_message from failed_chunks
	Model          string
	ProviderOff    bool // true when embedding provider is "off"
	StartedAt      time.Time
	FinishedAt     time.Time
}

// Evaluate applies the spec §3.2 behavior table to an IndexRun and updates
// the sentinel file accordingly. Callers print the banner (spec §3.3) when
// result.ShouldBanner is true and exit with result.ExitCode.
func (s *Store) Evaluate(run IndexRun) (*CheckResult, error) {
	panic("not implemented: internal/health.Store.Evaluate")
}

// PrintBanner writes the stderr banner from spec §3.3 to the given writer.
// Safe to call even when not unhealthy (it is a no-op in that case based
// on the CheckResult.ShouldBanner flag; callers should gate by that flag).
func PrintBanner(w io.Writer, run IndexRun, sentinelPath string) {
	panic("not implemented: internal/health.PrintBanner")
}
