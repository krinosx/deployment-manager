# Component 2: `gitutil` + `pipeline` (Updated — Done)

See `00-overview.md` for full context and current architecture status before making
decisions here. **This component is complete and tested, including against a real repo.**

## `internal/gitutil/gitutil.go`

```go
func RemoteHeadSHA(repoDir, branch string) (string, error)
func PullBranch(repoDir, branch string) error
```

- `RemoteHeadSHA` runs `git ls-remote origin <branch>`, parses the SHA from the output,
  touches no working-tree state.
- `PullBranch` runs `git pull --ff-only origin <branch>`, using `CombinedOutput()` (not
  just `Output()`) so stderr — where git's actual error detail lives — is captured, not
  just stdout. `--ff-only` was added deliberately so an unexpected diverged-history
  situation fails loudly instead of attempting an unattended merge.
- Both use `cmd.Dir = repoDir` explicitly rather than relying on process working
  directory, since this matters once running under systemd.

Tested in `gitutil_test.go` against **real git repos created inline per test** (a bare
repo acting as "origin," a working clone pushed to it, both via `t.TempDir()` — no
network access or external fixtures needed). Covers: happy path SHA retrieval, missing
branch error case, and a basic pull-succeeds case.

## `internal/pipeline/pipeline.go`

```go
type Config struct {
	RepoDir   string `json:"repo_dir"`
	SrcDir    string `json:"src_dir"`
	LibDir    string `json:"lib_dir"`
	BackupDir string `json:"backup_dir"`
}

type StageResult struct {
	Stage    string
	Success  bool
	Output   string
	Duration time.Duration
}

func RunBackup(cfg Config) StageResult
func RunPull(cfg Config, branch string) StageResult
func RunConfigure(cfg Config) StageResult
func RunMake(cfg Config) StageResult
```

Key implementation notes:
- **No `Branch` field on `pipeline.Config`** — deliberately removed after initially
  including it, once the decision was made that branch is an orchestration concern (see
  `00-overview.md`). `RunPull` takes `branch` as an explicit parameter instead.
- `RunBackup` shells out to `cp -r <lib_dir> <backup_dir>/lib-<timestamp>` — no manual
  recursive-copy code, no retention/pruning logic (explicitly out of scope, see open
  questions in the overview).
- `RunConfigure` runs `./configure` with `cmd.Dir = cfg.RepoDir`.
- `RunMake` runs `make clean` then `make -j4` as two separate `exec.Command` calls (not
  through a shell), both with `cmd.Dir = cfg.SrcDir`. If `make clean` fails, it returns
  immediately without attempting the build. **No binary-swap logic exists or is
  needed** — `make` itself produces `circle` directly in its final location per the
  project's actual Makefile behavior (confirmed by the user, not assumed).
- All four functions return a `StageResult`, never an `error` — `Success`/`Output`
  inside the struct is the sole error-signaling mechanism, keeping `deploy-check`'s
  orchestration loop simple (check `.Success`, no separate error path).
- **Known minor gap:** `RunBackup`, `RunConfigure`, and `RunMake` all populate `Output`
  with the command's real `CombinedOutput()` on success too, but `RunPull` only sets
  `Output` on error — a successful pull leaves it empty. This surfaced when
  `deploy-tui`'s log drill-down (component 5) started showing `Output` per stage: a
  passing `pull` stage always renders "(no output captured)" there, while `configure`
  and `make` show their real output. Not yet fixed, tracked in `00-overview.md`'s open
  questions.

Tested in `pipeline_test.go` against an **inline fake project fixture**
(`setupFakeProject`, via `t.TempDir()`): a real `configure` shell script, a real
`Makefile` with `all`/`clean` targets, and a real `lib/` directory with a dummy file —
covers `RunConfigure`, `RunMake`, and `RunBackup` (which additionally asserts the backed
up file actually landed in the destination, not just that `cp` exited 0).

## Nothing outstanding here

This component has since been proven against the user's real CircleMud repo via
`deploy-check` (component 3), not just the fake fixtures — a full real run (backup,
pull, configure, make) completed successfully. No further work is currently planned for
this package beyond the deferred backup-retention item tracked in the overview.
