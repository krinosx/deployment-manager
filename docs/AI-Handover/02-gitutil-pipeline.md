# Component 2: `gitutil` + `pipeline`

See `00-overview.md` for full context, architecture table, and cross-component
contracts before making decisions here.

## Scope

The actual work logic — independent of logging, state, CLI flags, or systemd. Should
be testable on its own (e.g. via a throwaway `main.go` or unit tests) before being
wired into the `deploy-check` binary in component 3.

## `internal/gitutil`

Wraps `git` via `os/exec` (no embedded git library — deliberate choice, see overview).

Needed functions (names indicative, not final):
- `RemoteHeadSHA(branch string) (string, error)` — runs `git ls-remote origin <branch>`,
  parses and returns the SHA, **without** touching the working tree. This is what makes
  the "check without deploying" step cheap.
- `Pull(branch string) error` — runs the actual `git pull` for the target branch.

## `internal/pipeline`

Encapsulates the deploy steps as discrete, individually-timeable stages, since component
3 logs one journal entry per stage (`backup`, `pull`, `configure`, `make`).

Needed shape (indicative):
```go
type StageResult struct {
    Stage    string // "backup" | "pull" | "configure" | "make"
    Success  bool
    Output   string // combined stdout+stderr, for local error reporting (not stored long-term — journal owns history)
    Duration time.Duration
}

func RunBackup(libPath string) StageResult
func RunPull(branch string) StageResult
func RunConfigure(projectRoot string) StageResult
func RunMake(srcPath string) StageResult
```

Each stage function should:
- Capture stdout+stderr combined (so failures are debuggable).
- Return success/failure via exit code check, not just "did the command run."
- Not itself write to the journal — that's the caller's job (component 3), keeping this
  package a pure, loggable, testable unit of work.

### Backup stage

- Straightforward copy/archive of `lib/` to a backup location. Naming scheme (timestamp?
  SHA-tagged?) and retention/pruning are open — currently just mirrors what the user does
  manually today. Worth deciding a simple retention policy here so backups don't
  silently fill the disk (flagged as an open question in the overview).

### Configure / Make stage

- Straightforward `os/exec` calls to `./configure` (project root) and
  `make clean && make -j4` (in `src/`). Working directory handling matters — make sure
  each `exec.Cmd` sets `Dir` explicitly rather than relying on process cwd.

### Binary swap

- After a successful `make`, the new `circle` binary should be moved/renamed into place
  atomically (e.g. build to a temp name, then `os.Rename` into `/bin/circle`). This is
  safe with the game running, per the "Key constraints & decisions" note in the overview
  (overwriting a running binary's file doesn't affect the already-running process).
- Whether to keep the previous binary as a rollback copy (`circle.bak` or SHA-tagged) is
  an open decision — flagged in the overview as a nice-to-have, not committed to yet.

## Open questions for this session

- Exact commands/flags for backup (tar? rsync? plain cp -r?) and where backups are stored.
- Whether `RunMake` should build to a staging path and only swap on success (recommended —
  keeps a bad build from ever touching the live `circle` binary), and where that staging
  path lives.
- Whether to keep N previous binaries for rollback, and how many.
- Error handling contract: should a stage function ever partially mutate state (e.g. a
  half-completed backup) that needs cleanup on failure?
