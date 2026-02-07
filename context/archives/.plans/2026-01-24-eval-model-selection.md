# Plan: Add Model Selection to Eval Runner

**Date:** 2026-01-24
**Branch:** `para/eval-model-selection`
**Objective:** Add configurable model selection to codetect-eval with sensible defaults to control costs

## Problem

The eval runner currently doesn't specify a model when calling `claude`, which means:
- It uses whatever the user's default Claude Code model is
- If the user has Opus as their default, eval runs become very expensive
- No way to force a consistent model across different users/environments
- No way to run cheaper evals with Haiku for bulk testing

## Solution

Add a `--model` flag to the eval runner that:
1. Defaults to `sonnet` (good balance of capability and cost)
2. Can be overridden to `haiku` for cheaper bulk testing
3. Can be set to `opus` if explicitly needed
4. Ensures consistent model usage across eval runs

## Implementation Steps

### Phase 1: Update Types and Config

**File:** `evals/types.go`

1. Add `Model` field to `EvalConfig` struct:
```go
type EvalConfig struct {
    RepoPath      string   `json:"repo_path"`
    Categories    []string `json:"categories,omitempty"`
    TestCaseIDs   []string `json:"test_case_ids,omitempty"`
    Parallel      int      `json:"parallel"`
    Timeout       time.Duration `json:"timeout"`
    OutputDir     string   `json:"output_dir"`
    Model         string   `json:"model"`           // NEW: Model to use (sonnet, haiku, opus)
    Verbose       bool     `json:"verbose"`
}
```

2. Update `DefaultConfig()` to set `Model: "sonnet"`:
```go
func DefaultConfig() EvalConfig {
    return EvalConfig{
        RepoPath:  ".",
        Parallel:  1,
        Timeout:   5 * time.Minute,
        OutputDir: "evals/results",
        Model:     "sonnet",  // NEW: Default to sonnet for cost control
        Verbose:   false,
    }
}
```

3. Add `Model` field to `EvalReport` so we track which model was used:
```go
type EvalReport struct {
    Timestamp   time.Time          `json:"timestamp"`
    Config      EvalConfig         `json:"config"`
    Summary     ReportSummary      `json:"summary"`
    Results     []ComparisonResult `json:"results"`
    RawResults  []RunResult        `json:"raw_results,omitempty"`
}
```

### Phase 2: Update Runner

**File:** `evals/runner.go`

1. Update `buildClaudeArgs()` to include `--model` flag:
```go
func (r *Runner) buildClaudeArgs(tc TestCase, mode ExecutionMode) []string {
    args := []string{
        "-p", tc.Prompt,
        "--output-format", "stream-json",
        "--verbose",
        "--model", r.config.Model,  // NEW: Specify model explicitly
    }

    // ... rest of function unchanged
}
```

### Phase 3: Update CLI

**File:** `cmd/codetect-eval/main.go`

1. Add `--model` flag to `runEval()` function:
```go
func runEval(args []string) {
    fs := flag.NewFlagSet("run", flag.ExitOnError)
    repoPath := fs.String("repo", ".", "Path to repository to evaluate")
    casesDir := fs.String("cases", "evals/cases", "Directory containing test case JSONL files")
    outputDir := fs.String("output", "evals/results", "Output directory for results")
    categories := fs.String("category", "", "Filter by category (comma-separated: search,navigate,understand)")
    parallel := fs.Int("parallel", 10, "Number of parallel test case executions (default: 10)")
    timeout := fs.Duration("timeout", 5*time.Minute, "Timeout per test case")
    model := fs.String("model", "sonnet", "Model to use (sonnet, haiku, opus)")  // NEW
    verbose := fs.Bool("verbose", false, "Verbose output")
    fs.Parse(args)

    config := evals.DefaultConfig()
    config.RepoPath = *repoPath
    config.OutputDir = *outputDir
    config.Parallel = *parallel
    config.Timeout = *timeout
    config.Model = *model      // NEW
    config.Verbose = *verbose

    // ... rest of function unchanged
}
```

2. Update `printUsage()` to document the new flag:
```go
Run Options:
  --repo <path>      Repository to evaluate (default: .)
  --cases <dir>      Test cases directory (default: evals/cases)
  --output <dir>     Output directory (default: evals/results)
  --category <cat>   Filter by category (search,navigate,understand)
  --parallel <n>     Number of parallel executions (default: 10)
  --timeout <dur>    Timeout per test case (default: 5m)
  --model <model>    Model to use: sonnet (default), haiku, opus    // NEW
  --verbose          Verbose output
```

### Phase 4: Update Reporter

**File:** `evals/report.go`

1. Update `PrintReportToStdout()` to show which model was used:
```go
func (r *Reporter) PrintReportToStdout(report *EvalReport) {
    fmt.Println("\n" + strings.Repeat("=", 80))
    fmt.Println("CODETECT EVAL REPORT")
    fmt.Println(strings.Repeat("=", 80))
    fmt.Printf("Timestamp: %s\n", report.Timestamp.Format("2006-01-02 15:04:05"))
    fmt.Printf("Repository: %s\n", report.Config.RepoPath)
    fmt.Printf("Model: %s\n", report.Config.Model)  // NEW
    // ... rest of function unchanged
}
```

## Testing Plan

1. **Test default behavior (sonnet):**
```bash
codetect-eval run --repo . --category search
# Should use sonnet model
```

2. **Test haiku override:**
```bash
codetect-eval run --repo . --category search --model haiku
# Should use haiku model for cheaper evals
```

3. **Test opus override:**
```bash
codetect-eval run --repo . --category search --model opus
# Should use opus model (explicit opt-in)
```

4. **Verify model appears in results:**
```bash
codetect-eval report
# Should show which model was used in the report
```

## Success Criteria

- ✅ Eval runner defaults to sonnet model
- ✅ `--model` flag allows switching between sonnet/haiku/opus
- ✅ Model is passed to `claude` CLI via `--model` flag
- ✅ Model used is tracked in eval results JSON
- ✅ Model is displayed in printed reports
- ✅ Documentation updated in help text

## Files to Modify

1. `evals/types.go` - Add Model field to EvalConfig and DefaultConfig
2. `evals/runner.go` - Add --model flag to buildClaudeArgs()
3. `cmd/codetect-eval/main.go` - Add --model CLI flag and update help
4. `evals/report.go` - Display model in report output

## Risks and Considerations

- **Breaking change:** Existing eval runs will now default to sonnet instead of user's default model
  - Mitigation: This is actually desired behavior for cost control
  - Document in CHANGELOG or release notes

- **Invalid model names:** User could pass an invalid model name
  - Mitigation: Let Claude CLI handle validation (will error if invalid)
  - Could add validation in future if needed

## Follow-up Work (Optional)

- Add model validation to provide better error messages
- Add model recommendation based on eval size (haiku for >50 tests, etc.)
- Track model in per-test-case results for hybrid eval runs
