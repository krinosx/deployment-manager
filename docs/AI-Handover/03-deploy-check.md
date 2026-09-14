# Component 3: `deploy-check` Binary (Updated — Done)

See `00-overview.md` for full context, the complete orchestration flow, and current
architecture status before making decisions here. **This component is complete, tested
against a real repo, and installed/verified under systemd.**

## Dependencies

- `internal/gitutil`, `internal/pipeline`, `internal/journallog`, `internal/config`
- `github.com/coreos/go-systemd/v22/journal` (via `journallog`) — **note:** the fields
  map passed to `journal.Send` must include `"SYSLOG_IDENTIFIER": "deploy-check"`
  explicitly; this was missing initially and caused `journalctl -t deploy-check` to miss
  all per-stage log lines (they were only visible via `-u deploy-check.service`). Fixed
  and verified — see `00-overview.md` for full detail.
- `github.com/google/uuid` — added specifically to generate one `DEPLOY_RUN_ID` per
  execution (see below and `00-overview.md`)

## CLI flags (as implemented, not just proposed)

- `--config <path>` — required in practice; no hardcoded default fallback currently
  relied upon in real usage (the systemd unit always passes it explicitly).
- `--dry-run` — checks remote SHA, prints what would be deployed, does not run the
  pipeline or touch state.
- `--force` — runs the pipeline even if remote SHA equals last-deployed SHA.

## Orchestration flow

See `00-overview.md`'s "`deploy-check` orchestration flow" section for the full
numbered sequence — it's kept there as the single source of truth since it's a
cross-cutting detail relevant to component 5 as well (the TUI reads what this binary
writes).

## Key implementation details worth knowing for future sessions

- **Locking**: `syscall.Flock` on `<state_file_path>.lock`, `LOCK_EX|LOCK_NB` (exclusive,
  non-blocking). A second concurrent invocation fails fast with an stderr message and
  exit code 1, rather than queueing. The lock is released implicitly when the file
  descriptor closes (`defer lockFile.Close()`), no explicit unlock call needed.
- **State file**: read via `os.ReadFile` (missing file treated as empty SHA, not an
  error — handles the very first run gracefully). Written via a temp-file-then-rename
  pattern (`os.Rename` is atomic on Linux), so a crash mid-write can't corrupt the state
  file.
- **Run ID**: `uuid.NewString()` is called once near the top of `main()`, after the
  no-op/dry-run checks (i.e. only real pipeline runs get a run ID — no-op checks don't).
  This ID is threaded through `runPipeline(cfg, runID, remoteSHA)` and included in every
  `journallog.LogStage` call for that execution. This was a deliberate addition made
  *before* building `deploy-tui`, specifically to give the history view an unambiguous
  grouping key instead of relying on SHA+timestamp proximity.
- **`runPipeline`'s stage list is built as a slice of closures**, with the backup stage
  conditionally appended only if `cfg.EnableBackup` is true — this is how the
  configurable-backup requirement was actually wired in, not via an `if` check inside a
  fixed-stage loop.
- Exit codes matter operationally: non-zero exit is what makes a failed run show as
  `failed` in `systemctl status deploy-check.service`, independent of journal detail.

## Verified against real usage

- Manual `-dry-run` and real (non-dry-run) runs against the user's actual CircleMud repo
  both completed successfully — full backup → pull → configure → make cycle confirmed
  working, including subsequent no-op runs correctly detecting "nothing new" on repeat
  invocation.
- Re-verified end-to-end again after the `DEPLOY_RUN_ID` addition, confirming all four
  stages in a single run share the same run ID via `journalctl -o verbose`.
- Re-verified again once installed under systemd (see `04-systemd-units.md`) —
  `sudo systemctl start deploy-check.service` produces the expected journal output.

## Remaining open items (tracked centrally in `00-overview.md`)

- No journal entry currently written for no-op ("nothing to deploy") runs.
- No journal entry written when lock acquisition fails.
- No automatic creation of missing parent directories for `backup_dir` /
  `state_file_path` — currently a hard failure if they don't exist.
- No rollback/cleanup attempted on a failed stage.

None of these block `deploy-tui` work; they're independent hardening items that can be
picked up at any time.
