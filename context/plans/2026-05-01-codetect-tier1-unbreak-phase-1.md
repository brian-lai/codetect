# Phase 1 — Binary Collapse + Deprecation Shims

**Master:** `context/plans/2026-05-01-codetect-tier1-unbreak.md`
**Spec:** `context/data/2026-05-01-codetect-tier1-unbreak-spec.md` §1
**Branch:** `para/tier1-phase1-binary-collapse`

---

## Objective

Replace the five-binary surface (`codetect`, `codetect-index`, `codetect-daemon`, `codetect-eval`, `migrate-to-postgres`) with a single `codetect` binary that takes subcommands. `codetect-eval` stays as-is (dev tool). Install deprecation shim scripts so existing users' shell invocations of the old binary names keep working for one release with a visible warning.

**After this phase merges:** all documented `codetect <subcommand>` invocations work; the v2 indexer is reachable as `codetect index`; the daemon is reachable as `codetect daemon start`. Nothing about the index format or the MCP tool output changes.

---

## Files Touched

| Path | Action |
|---|---|
| `cmd/codetect/main.go` | Replace body with 10-line `os.Exit(int(commands.Dispatch(os.Args[1:])))` |
| `cmd/codetect/commands/commands.go` | Fill stubs (dispatch + help already in place) |
| `cmd/codetect/commands/mcp.go` | New — move MCP server startup here |
| `cmd/codetect/commands/init.go` | New — writes `.mcp.json` from `templates/mcp.json` |
| `cmd/codetect/commands/index.go` | New — body of today's `cmd/codetect-index/main.go:runIndex` |
| `cmd/codetect/commands/embed.go` | New — body of `runEmbed` |
| `cmd/codetect/commands/stats.go` | New — body of `runStats` |
| `cmd/codetect/commands/doctor.go` | New — body of `runDoctor` (Phase 3 extends) |
| `cmd/codetect/commands/daemon.go` | New — dispatches to daemon package with `start\|stop\|status\|logs` |
| `cmd/codetect/commands/registry.go` | New — dispatches to registry package |
| `cmd/codetect/commands/migrate.go` | New — body of `cmd/migrate-to-postgres/main.go` |
| `cmd/codetect/commands/version.go` | New — owns the `serverVersion` constant, exposes it to `mcp.go` |
| `cmd/codetect/commands/*_test.go` | New — dispatch + per-subcommand smoke tests |
| `cmd/codetect-index/` | Deleted |
| `cmd/codetect-daemon/` | Deleted |
| `cmd/migrate-to-postgres/` | Deleted |
| `scripts/shims/codetect-index.sh` | Fill stub (already in place) |
| `scripts/shims/codetect-daemon.sh` | Fill stub |
| `scripts/shims/migrate-to-postgres.sh` | Fill stub |
| `Makefile` | Build only `codetect` + `codetect-eval`; install shims |
| `install.sh` | Remove references to the three deleted binaries; add shim install step |
| `README.md` | Remove the invented CLI claims; fold the real CLI reference in |
| `docs/installation.md` | Same |
| `templates/mcp.json` | Unchanged (already correct — `{"command":"codetect","args":["mcp"]}`) |

---

## Architecture Decisions (local to this phase)

| Decision | Choice | Rationale |
|---|---|---|
| Code move vs. rewrite | Pure code move (copy function bodies, adjust imports, no logic changes) | Preserves behavior, preserves tests, smallest possible diff, easiest to review. |
| Test location | Tests for each moved command live next to their new file (`commands/index_test.go`) | Keeps subcommand tests with the code they test; original `cmd/codetect-index/*_test.go` moves with the function bodies. |
| Shim install mechanism | `make install` copies three `scripts/shims/*.sh` files to `$PREFIX/bin/` with `chmod +x` | Same mechanism as the main binary install; no per-OS special handling needed. |
| `codetect` with no args | Starts MCP server (preserves current behavior from the one thing the old binary did right) | Backward-compatible for anyone who already runs `codetect` from an `.mcp.json`. |

---

## Interface Boundaries (this phase)

| # | Boundary | Contract | Test name |
|---|---|---|---|
| B1 | Argv dispatch | `commands.Dispatch(argv []string) ExitCode` | `TestDispatch_KnownSubcommands`, `TestDispatch_UnknownExitsTwo`, `TestDispatch_NoArgsRunsMCP` |
| B2 | Shim invocation | `codetect-index foo` → stderr warning + `exec codetect index foo` | `TestShim_CodetectIndex_Delegates`, `TestShim_PrintsDeprecationWarning` |
| B7' | Daemon re-index invocation | `exec.Command("codetect", "index", project)` (was `"codetect-index"`) | `TestDaemon_InvokesCodetectIndex` (update of existing test) |

---

## Graceful Degradation (this phase)

| Scenario | Behavior |
|---|---|
| User has old `codetect-index` binary on PATH before new shim | Whichever is earlier on PATH wins. Shim install into the same dir as main binary means upgrade replaces the old binary. |
| `codetect` called with `--help` (Unix convention) | Routed to `help` → PrintHelp → exit 0. |
| `codetect --version` | Routed to `version` → prints + exit 0. |
| `codetect mcp` called interactively (TTY stdin) | Same as today: reads stdin; blocks on empty line; user Ctrl-C exits. Not a regression. |
| Old binary still present after upgrade (e.g., user had both a homebrew install and a manual install) | Install script logs `warning: found lingering codetect-index at /path; consider deleting` and continues. |

---

## Implementation Steps (TDD-first)

### Test-first skeleton (lands before any implementation code)

- [ ] test(e2e): add fresh-install golden-path test skeleton — red until this phase ships
  - Creates `cmd/codetect/commands/e2e_test.go::TestE2E_FreshInstallGoldenPath` per the Integration Test section below. Initially red (RunInit and RunIndex stubs panic). Goes green as the last step of this phase. Serves as the spine test extended by phases 2 & 3.
  - Tests: `TestE2E_FreshInstallGoldenPath` (red initially).

- [ ] test(e2e): add deprecated-binaries acceptance test skeleton — red until shims land
  - Creates `cmd/codetect/commands/e2e_test.go::TestE2E_DeprecatedBinariesStillWork` per the Acceptance Test section. Uses `make install PREFIX=$tmpdir`; invokes each shim via path; asserts exit code + stderr warning + passthrough.
  - Tests: `TestE2E_DeprecatedBinariesStillWork` (red initially).

- [ ] test(commands): add Dispatch table-test skeleton covering all subcommand routes
  - Creates `cmd/codetect/commands/commands_test.go` with `TestDispatch_Routes` table covering every spec §1.2 row. All subcommand functions still `panic`; test uses `defer recover()` to assert the panic message mentions "not implemented: <expected>". Confirms the dispatch table is wired correctly before any real logic moves.
  - Tests: `TestDispatch_Routes` (table with 14 rows from spec §1.2); `TestDispatch_UnknownExitsTwo`; `TestDispatch_NoArgsRoutesToMCP`.

- [ ] test(shims): add shim behavior e2e test skeleton
  - Creates `scripts/shims/shim_test.sh` (bash) run under `go test` via a `cmd/codetect/commands/shims_e2e_test.go` wrapper. Asserts: shim exits with same code as the wrapped command; stderr contains deprecation warning; arguments pass through verbatim.
  - Tests: `TestShim_CodetectIndex_Delegates`, `TestShim_CodetectDaemon_Delegates`, `TestShim_MigrateToPostgres_Delegates`, `TestShim_PrintsDeprecationWarning`.

### Move cmd/codetect-index → cmd/codetect/commands

- [ ] refactor(commands): move runIndex + its tests from cmd/codetect-index into commands/index.go
  - Copies `cmd/codetect-index/main.go:runIndex` body into `commands.RunIndex`; fixes imports; preserves flag definitions verbatim. Co-moves `index_test.go` if one exists.
  - Tests: existing `cmd/codetect-index/*_test.go` tests pass unchanged under their new location; `TestDispatch_Routes` subrow for `index` now reaches RunIndex without panic.

- [ ] refactor(commands): move runEmbed into commands/embed.go
  - Same mechanical move for `runEmbed`.
  - Tests: `TestRunEmbed_FlagParsing`, `TestDispatch_Routes[embed]`.

- [ ] refactor(commands): move runStats into commands/stats.go
  - Tests: `TestRunStats_JSONFlag`, `TestDispatch_Routes[stats]`.

- [ ] refactor(commands): move runDoctor into commands/doctor.go
  - Phase 3 extends this; for now a verbatim move of today's behavior.
  - Tests: `TestRunDoctor_NoIndex_Exits1`, `TestDispatch_Routes[doctor]`.

- [ ] refactor(commands): move runRegistry into commands/registry.go
  - `commands.RunRegistry(args)` dispatches to list/add/remove/stats from today's `cmd/codetect-index/main.go:runRegistry*` functions. Thin wrapper around `internal/registry` package.
  - Tests: `TestDispatch_Routes[registry]`; `TestRunRegistry_List_PrintsProjects`; `TestRunRegistry_Add_AddsToRegistry`.

- [ ] refactor(commands): move runVersion into commands/version.go; export `Version` const
  - Phase 1 needs the version string in two places (MCP `InitializeResult` and `codetect version`). Export `commands.Version = "3.8.0"`.
  - Tests: `TestRunVersion_PrintsVersionAndExits0`.

### Move cmd/codetect-daemon → cmd/codetect/commands/daemon.go

- [ ] refactor(commands): add daemon.go with dispatch to start/stop/status
  - `commands.RunDaemon(args)` dispatches on `args[0]` into the existing daemon package's functions (`start`, `stop`, `status` — the three subcommands that exist today at `cmd/codetect-daemon/main.go:29-33`). Thin wrapper; all the logic stays in `internal/daemon`. `logs` is explicitly NOT added in this phase — reviewer flagged it as net-new and it's scoped to §8.
  - Tests: `TestDispatch_Routes[daemon]`; `TestRunDaemon_Start_StartsProcess` (uses existing daemon test harness); `TestRunDaemon_Logs_ExitsUnknown` — asserts `codetect daemon logs` exits with "unknown daemon subcommand" until §8 adds it.

- [ ] test(daemon): add first-ever daemon test covering the runIndex invocation
  - Reviewer-flagged: no tests exist in `internal/daemon/` today (verified via `ls`). This phase creates the test file. Use a fake exec hook (inject `execFn func(name string, args ...string) *exec.Cmd` onto the Daemon struct, defaulting to `exec.CommandContext`) so the test can capture argv without spawning a real process.
  - Tests: `TestDaemon_InvokesCodetectIndex_WithCorrectArgv` — asserts the hook is called with `("codetect", "index", projectPath)`, NOT `("codetect-index", ...)`. Runs the daemon's `runIndex` function with a stub registry.

- [ ] fix(daemon): update exec.Command target from "codetect-index" to "codetect"
  - `internal/daemon/daemon.go:344`: `exec.CommandContext(d.ctx, "codetect", "index", projectPath)`. Boundary B7 in master plan. Requires the exec hook added in the previous step for testability.
  - Tests: `TestDaemon_InvokesCodetectIndex_WithCorrectArgv` goes green.

### Move cmd/migrate-to-postgres → cmd/codetect/commands/migrate.go

- [ ] refactor(commands): move migrate-to-postgres main into commands/migrate.go
  - Same mechanical move. The subcommand shows up as `codetect migrate-to-postgres` (with hyphens preserved as the intentional, Google-style-guide-blessed "move verb").
  - Tests: `TestDispatch_Routes[migrate-to-postgres]`; existing migrate tests pass.

### Fill new subcommands: mcp + init

- [ ] feat(commands): implement RunMCP (start MCP server on stdio, same as today's cmd/codetect/main)
  - Body is the 20 lines currently in `cmd/codetect/main.go`. No behavior change beyond moving.
  - Tests: `TestRunMCP_ReadsStdinAndExits` (feeds an empty stdin; expects exit 0); `TestRunMCP_CleanEOF_NoStderrOutput` — feeds empty stdin; captures stderr; asserts stderr is empty (prevents leaking a banner or deprecation warning into MCP clients that tail stderr).

- [ ] feat(commands): implement RunInit — write .mcp.json from templates/mcp.json
  - Embeds `templates/mcp.json` via `//go:embed`. Writes to `./.mcp.json`. Flag `--force` overwrites.
  - Tests: `TestRunInit_CreatesMcpJson`; `TestRunInit_ExistingFile_Exits1`; `TestRunInit_Force_Overwrites`.

### Rewrite entry point

- [ ] feat(cmd/codetect): rewrite main.go to dispatch via commands.Dispatch
  - `cmd/codetect/main.go` becomes 10 lines: `func main(){ os.Exit(int(commands.Dispatch(os.Args[1:]))) }`.
  - Tests: existing `TestDispatch_*` pass; new `TestMain_DispatchesToCommands` running the binary with `-v` flag.

### Shims + install

- [ ] feat(scripts/shims): implement all three shim scripts (deprecation warning + exec)
  - Files already stubbed by /para:plan; this step is verifying content and chmod +x.
  - Tests: `TestShim_*` (the e2e skeleton from step 2 now goes green).

- [ ] build(makefile): produce only codetect and codetect-eval; install shims alongside
  - `make build`: removes `go build -o dist/codetect-index ./cmd/codetect-index`, same for daemon and migrate. Keeps codetect + codetect-eval.
  - `make install`: copies `dist/codetect` and `dist/codetect-eval` to `$PREFIX/bin/`; copies `scripts/shims/*.sh` to `$PREFIX/bin/` (stripping `.sh` suffix) with `chmod +x`.
  - Tests: `make build && make install PREFIX=/tmp/codetect-install && /tmp/codetect-install/bin/codetect-index --help` exits 0 and stderr contains deprecation warning.

### Delete old cmd/ directories

- [ ] chore: delete cmd/codetect-index, cmd/codetect-daemon, cmd/migrate-to-postgres directories
  - Only happens after all tests above pass. Tests already co-located into commands.
  - Tests: `go build ./...` and `go test ./...` pass; `find cmd -type d -name 'codetect-*'` matches only `codetect` and `codetect-eval`.

### Docs

- [ ] docs(readme): replace imaginary CLI section with accurate unified-binary docs
  - README §"CLI Commands" now reflects the real subcommand list; removes mentions of `codetect-index` / `codetect-daemon` / `migrate-to-postgres` except in a "Deprecation" callout.
  - Tests: a `docs/lint_test.sh` (new) greps README for `codetect-index` and requires it appears only inside the Deprecation callout.

- [ ] docs(installation): update installation.md with shim behavior and one-line upgrade instructions
  - Documents: `make install` now installs shims; shims warn on stderr and delegate; shims removed in v4.0.
  - Tests: doc lint script covers installation.md too.

---

## Test Suites Added This Phase (from Progressive Regression Rule)

1. `cmd/codetect/commands/commands_test.go` — `TestDispatch_*`
2. `cmd/codetect/commands/<subcmd>_test.go` — one file per moved subcommand; tests moved with bodies
3. `cmd/codetect/commands/shims_e2e_test.go` — driving `scripts/shims/shim_test.sh`
4. `docs/lint_test.sh` — invoked from `go test ./...` via a simple `TestDocsLint` wrapper in `cmd/codetect/commands/docs_lint_test.go`

---

## Unit Tests Inventory

```
TestDispatch_Routes (14 rows)
  - "mcp"              → RunMCP
  - "serve"            → RunMCP
  - "init"             → RunInit
  - "index"            → RunIndex
  - "embed"            → RunEmbed
  - "stats"            → RunStats
  - "doctor"           → RunDoctor
  - "daemon"           → RunDaemon
  - "registry"         → RunRegistry
  - "migrate-to-postgres" → RunMigrate
  - "version"          → RunVersion
  - "-v"               → RunVersion
  - "--version"        → RunVersion
  - "help"             → PrintHelp + exit 0
TestDispatch_NoArgsRoutesToMCP
TestDispatch_UnknownExitsTwo
  - args: ["nonsense"] → exit 2, stderr contains "unknown subcommand"

TestRunInit_CreatesMcpJson
TestRunInit_ExistingFile_Exits1
TestRunInit_Force_Overwrites
TestRunVersion_PrintsVersionAndExits0
TestRunMCP_ReadsStdinAndExits

TestShim_CodetectIndex_Delegates
TestShim_CodetectDaemon_Delegates
TestShim_MigrateToPostgres_Delegates
TestShim_PrintsDeprecationWarning
TestShim_PassesArgumentsVerbatim

TestDaemon_InvokesCodetectIndex  (update: argv assertion changes)
```

## Integration Test

`cmd/codetect/commands/e2e_test.go::TestE2E_FreshInstallGoldenPath`:

```
1. tmpdir = t.TempDir()
2. cp -r ./testdata/tiny-go-repo tmpdir/repo
3. cd tmpdir/repo
4. codetect init → asserts .mcp.json exists
5. codetect index → asserts exit 0 (symbols not yet populated until phase 2; ok for phase 1)
6. codetect stats --json → asserts JSON parses, contains "chunks_created" > 0
7. codetect version → asserts prints version and exits 0
```

This test is the spine for all future phases; phase 2 extends step 5's assertions to cover symbols.

## Acceptance Test

`cmd/codetect/commands/e2e_test.go::TestE2E_DeprecatedBinariesStillWork`:

```
1. make install PREFIX=$TMPDIR/install
2. PATH=$TMPDIR/install/bin:$PATH codetect-index --help
   → asserts exit 0
   → asserts stderr contains "'codetect-index' is deprecated"
   → asserts stdout contains the index command's help
3. Same for codetect-daemon, migrate-to-postgres.
```

---

## Success Criteria

- [ ] `make build` produces exactly `dist/codetect` and `dist/codetect-eval`; no other binaries.
- [ ] `codetect help` prints help and exits 0.
- [ ] `codetect unknown` prints an error and exits 2.
- [ ] `codetect` with no args starts the MCP server (same as `codetect mcp`).
- [ ] `codetect-index --help` still works via shim with stderr deprecation warning.
- [ ] `codetect daemon start` starts the daemon (same behavior as old `codetect-daemon start`).
- [ ] `internal/daemon/daemon.go:344` invokes `codetect index`, not `codetect-index index`.
- [ ] `TestE2E_FreshInstallGoldenPath` is green.
- [ ] Total line count reduction: at least -200 LOC (measured by `git diff --shortstat main..HEAD`).

## Risks Specific to This Phase

- **go:embed of templates/mcp.json** requires the file to be in the same module; this is already true, but we add the embed directive in a new location — run `go build` to confirm.
- **`make install`'s `PREFIX` handling** varies across systems; this phase must not regress the existing behavior. Run `make install PREFIX=/tmp/foo` on both Linux and macOS in CI.
- **Renaming `cmd/codetect-index` tests** could lose history. Use `git mv` (not delete + create) to preserve blame.
- **Homebrew users bypass the shim install path.** `brew install` writes into `/opt/homebrew/Cellar/codetect/<version>/bin/` and symlinks; our `make install` shims land wherever `$PREFIX/bin` is. Homebrew's upgrade flow (`brew cleanup`) removes files not in the formula's explicit install list. **Decision (round-2 review):** this is blocking, not nice-to-have. Ship a **new implementation step** in this phase:

  - [ ] build(homebrew): update the Homebrew formula to install shims alongside the main binary. Formula lives in a separate tap repo (not this repo); this phase ships a PR against that tap OR, if tap maintenance is out of scope for Tier 1, prominent MIGRATION.md warning that the old binary names don't work under brew until the formula is updated.

  `TestE2E_DeprecatedBinariesStillWork` covers `make install` only; add `TestE2E_Homebrew_ShimsPresent` as a CI-only test gated on a `BREW_PREFIX` env var that's set in the tap's CI.
- **Old binary cached elsewhere on PATH.** If the user previously installed via a different mechanism (e.g., `go install`) and the old `codetect-index` is on PATH earlier than the new shim, the old binary (pre-3.8) will run. `install.sh` logs a warning but continues. To be safer, add a `--fail-on-conflict` flag to install.sh that exits non-zero if a pre-3.8 binary is on PATH.

## Out of Scope for This Phase

- Any change to what `codetect index` produces (that's phase 2).
- Any health check logic (that's phase 3).
- Deleting the v1 indexer code (that's phase 4).
- Running the indexer in-process from the daemon (that's §8 in a future plan).
