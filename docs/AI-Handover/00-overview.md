# Deploy-Manager — Project Handoff / Overview (Updated)

This document supersedes the earlier `00-overview.md`. It reflects actual implementation
progress, not just the original plan — components 1 through 4 are built, tested, and
verified against a real repo and a real systemd installation. Component 5 (`deploy-tui`)
is in progress. Paste this file into any new session before continuing work, along with
the component-specific doc for whatever you're working on.

## Background / Problem (unchanged)

- User runs a **CircleMud** game server (C codebase) on a resource-constrained Ubuntu
  Linux server. Two instances run concurrently. RAM is tight.
- Manual deploy process being automated: backup `lib/` → `git pull` a specific branch →
  `./configure` → `cd src && make clean && make -j4` (produces `circle` binary directly
  in its final `/bin` location — **no separate binary-swap step needed**, `make` handles
  this itself).
- **Game restart stays manual, by design.** An admin logs into the running game and
  issues an in-game restart command, because tests may be running against a live
  instance at deploy time. This is safe because overwriting a running binary's file on
  Linux does not affect the already-running process (old inode stays open via the
  process's existing file descriptor). `deploy-check` never restarts the game.

## Actual environment

- Developer machine: **Ubuntu 26.04**, used both for development and as the real test
  target — systemd/journald available locally, so integration was verified for real,
  not just stubbed.
- IDE: **GoLand** (JetBrains).
- Go version: **1.26.7** (chosen over 1.27.0 for maturity; no functional difference for
  this project either way — easy to change later via `go.mod`).
- Real project paths used for testing (specific to the developer's machine, not a
  general requirement):
  - Repo: `/home/krinosx/projetos/debo/debo-priv` (branch `master`)
  - Lib data: `/home/krinosx/projetos/debo/debo-lib` (symlinked as `lib` inside the repo)
  - Backups: originally `/tmp/backup`, later moved to a persistent path (see Config below)
- Code is versioned on GitHub: `github.com/krinosx/deployment-manager`, local path
  `/home/krinosx/GolandProjects/deployment-manager`.

## Chosen tech stack (unchanged, now proven in practice)

- **Language: Go** for both `deploy-check` and `deploy-tui`.
- `os/exec` used throughout to shell out to `git`, `./configure`, `make`, `cp` — no
  embedded git library, no shell-string concatenation (each command invoked directly,
  not via `sh -c`, avoiding quoting/injection concerns even though inputs are fixed).
- **Logging: real systemd journal**, via `github.com/coreos/go-systemd/v22/journal`.
  This was originally stubbed to stdout, then swapped to the real implementation and
  verified via `journalctl --user -f -o verbose` — **this swap is complete**, not
  pending. **Important fix applied since:** the fields map passed to `journal.Send`
  must explicitly include `"SYSLOG_IDENTIFIER": "deploy-check"`. Without it,
  `journal.Send` entries are tagged correctly for `_SYSTEMD_UNIT`-based filtering
  (`journalctl -u deploy-check.service`, since systemd attaches that automatically to
  everything in the unit's cgroup) but are **invisible** to tag-based filtering
  (`journalctl -t deploy-check`), because `go-systemd/journal` does not set
  `SYSLOG_IDENTIFIER` automatically the way stdout output does. This matters a lot
  because `internal/history` (component 5) filters via `-t deploy-check` specifically
  so it works whether or not `deploy-check` is running under systemd at all — this fix
  is required for that to function correctly.
- **Scheduling: systemd timer, no resident daemon.** `deploy-check` is a one-shot binary
  invoked by a `.timer` unit; it runs and exits. **Installed and verified as a
  system-wide unit** (`/etc/systemd/system/`, not a user unit) — chosen because the tool
  should keep working regardless of whether anyone is logged in interactively.
- **TUI: bubbletea + lipgloss.** A minimal "hello world" bubbletea skeleton has been
  built and run successfully (renders text, quits on `q`). Real data wiring is in
  progress (see component 5 doc).
- **Run identification: `github.com/google/uuid`.** Added specifically so every
  `deploy-check` execution generates one UUID (`DEPLOY_RUN_ID`) shared by all of that
  run's stage log entries — added *before* building `deploy-tui`'s history view,
  specifically to avoid a fragile SHA+timestamp grouping heuristic.
- **State: minimal**, a single `last-deployed-sha` file, path fully configurable (see
  Config below), written atomically (temp file + `os.Rename`).
- **Config: JSON file**, loaded at startup via `internal/config`, path passed via a
  `--config` CLI flag (not hardcoded, not env-var based). This was a deliberate choice
  made early, specifically so paths and the backup on/off toggle never need to be
  hardcoded in Go source.

## Architecture summary (updated status)

| # | Component | Stack | Status |
|---|---|---|---|
| 1 | Project skeleton + `internal/config` | Go module | **Done.** See `01-project-skeleton.md`. |
| 2 | `gitutil` + `pipeline` | Go, `os/exec` | **Done, tested.** See `02-gitutil-pipeline.md`. |
| 3 | `deploy-check` binary | Go + `go-systemd/journal` + `google/uuid` | **Done, tested against real repo.** See `03-deploy-check.md`. |
| 4 | systemd `.service` + `.timer` units | systemd unit files | **Done, installed and verified.** See `04-systemd-units.md`. |
| 5 | `deploy-tui` | Go, bubbletea + lipgloss | **In progress.** See `05-deploy-tui.md`. |

## Final package layout (as built)

```
deployment-manager/
  go.mod
  go.sum
  cmd/
    deploy-check/
      main.go
    deploy-tui/
      main.go
  internal/
    gitutil/
      gitutil.go
      gitutil_test.go
    pipeline/
      pipeline.go
      pipeline_test.go
    journallog/
      journallog.go
    config/
      config.go
      config_test.go
      testdata/
        config.json
    history/
      history.go        <- new, component 5, fetches + parses journal entries
```

## Config file shape (as implemented)

```json
{
  "pipeline": {
    "repo_dir": "/home/krinosx/projetos/debo/debo-priv",
    "src_dir": "/home/krinosx/projetos/debo/debo-priv/src",
    "lib_dir": "/home/krinosx/projetos/debo/debo-lib",
    "backup_dir": "/var/lib/deploy-manager/backups"
  },
  "branch": "master",
  "enable_backup": true,
  "state_file_path": "/var/lib/deploy-manager/last-deployed-sha"
}
```

Notes on this shape:
- `Branch` deliberately lives at the top level of `config.Config`, **not** inside
  `pipeline.Config` — it's an orchestration/"what to deploy" decision, not a filesystem
  detail. `pipeline.RunPull` takes `branch` as an explicit second parameter as a result.
- `EnableBackup` lives at the top level too, read by `deploy-check`'s orchestration loop
  (`runPipeline`) to decide whether to include the backup stage at all — `pipeline.RunBackup`
  itself has no awareness of whether it's "enabled," it just does the copy when called.
  This was an explicit ask from the user: **the person configuring the app should be
  able to turn backups on/off** without code changes.
- Both `backup_dir` and `state_file_path` were originally tested under `/tmp/...` during
  early manual testing, then deliberately moved to persistent paths under
  `/var/lib/deploy-manager/` before the systemd install, since `/tmp` is cleared on
  reboot on Ubuntu and would silently reset "last deployed" state.

## Standardized journal fields (final, includes RunID addition)

Written by `journallog.LogStage`, one call per pipeline stage (`backup`, `pull`,
`configure`, `make`), filtered via `journalctl -t deploy-check` (matches
`SyslogIdentifier=deploy-check` in the systemd unit):

- `DEPLOY_RUN_ID` — a UUID (via `google/uuid`), generated once per `deploy-check`
  execution in `main()`, shared by every stage log entry from that run. **Added
  specifically to support unambiguous grouping in `deploy-tui`'s history view** — added
  proactively before building that view, not retrofitted after hitting a grouping bug.
- `DEPLOY_STAGE` — one of: `backup`, `pull`, `configure`, `make`
- `DEPLOY_STATUS` — one of: `success`, `failure` (no `start` status currently emitted —
  see open questions)
- `DEPLOY_SHA` — the commit SHA being deployed
- `DEPLOY_DURATION_MS` — stage duration in milliseconds, as a string (all journal field
  values are strings; convert with `strconv` where a numeric value is needed)
- `SYSLOG_IDENTIFIER` — explicitly set to `"deploy-check"` in every call (see fix above);
  not one of the custom `DEPLOY_*` fields, but required for `-t deploy-check` filtering
  to work at all.

Priority mapping: `journal.PriInfo` for success, `journal.PriErr` for failure — chosen so
`journalctl -p err -u deploy-check` (or `-t deploy-check`) surfaces failed stages
directly without needing to filter on `DEPLOY_STATUS` manually.

## `deploy-check` orchestration flow (as implemented)

1. Parse flags: `--config <path>`, `--dry-run`, `--force`.
2. Load `config.Config` from the given path.
3. Acquire an exclusive, non-blocking `flock` on `<state_file_path>.lock` (via
   `syscall.Flock`). If already locked, print an error and exit 1 — no queueing/waiting.
4. Read `last-deployed-sha` from `state_file_path` (missing file = treated as empty,
   i.e. "no prior deploy").
5. `gitutil.RemoteHeadSHA(repoDir, branch)` to get the current remote SHA — no working
   tree changes yet.
6. If `remoteSHA == lastSHA` and not `--force`: print "no new commits" and exit 0
   cleanly. **No journal entry is written for a no-op run** — see open questions.
7. If `--dry-run`: print what would be deployed and exit, without running anything.
8. Otherwise, generate a `runID` (UUID) and run stages in order via `runPipeline`:
   `backup` (only if `cfg.EnableBackup`) → `pull` → `configure` → `make`. Each stage
   result is logged via `journallog.LogStage(runID, stage, status, sha, durationMs)`.
   Stops at the first failing stage (does not attempt later stages or any cleanup).
9. On full success: atomically overwrite `state_file_path` with the new SHA, print
   success, exit 0. On any stage failure: exit 1, `last-deployed-sha` remains unchanged.

## systemd units (as installed)

Installed system-wide (chosen deliberately over user units, since the tool should run
regardless of interactive login state):

- `/etc/systemd/system/deploy-check.service` — `Type=oneshot`, runs as the developer's
  own non-root user (`User=krinosx`), `SyslogIdentifier=deploy-check`,
  `Wants=network-online.target` / `After=network-online.target` (added because
  `deploy-check` needs network access for git operations, and a boot-time run could
  otherwise race against networking not being up yet).
- `/etc/systemd/system/deploy-check.timer` — `OnBootSec=2min`, `OnUnitActiveSec=15min`,
  `Persistent=true` (a missed run due to downtime fires once at next boot).
- Binary installed to `/usr/local/bin/deploy-check`; config at
  `/etc/deploy-manager/config.json`.
- Verified working end-to-end: `sudo systemctl start deploy-check.service` triggers a
  real run, visible via `journalctl -u deploy-check` / `journalctl -t deploy-check`.

## Open questions / not yet decided (updated)

Carried over from before, still open:
- **Backup retention/pruning** — explicitly out of scope for now. The person
  configuring the app being able to choose whether backups run at all is **already
  implemented** via `EnableBackup` in the config. Actual pruning of old backup folders
  is still unimplemented and deliberately deferred.
- Locking: current behavior on lock failure is silent exit 1 with an stderr message, no
  journal entry — whether that should also log to the journal is still open.
- No-op runs (checked, nothing new to deploy) do not currently produce any journal
  entry — open question about whether they should, at what priority, to help confirm the
  timer is actually firing on schedule.
- Whether a failed stage should attempt any rollback/cleanup — currently does not; state
  is left as-is for manual inspection.
- Missing parent directories for `backup_dir` or `state_file_path` currently cause a hard
  failure (`cp`/`os.WriteFile` don't create missing parents) rather than being
  auto-created — flagged as a small hardening item, not yet done.

New, specific to `deploy-tui` (see `05-deploy-tui.md` for full detail):
- Permission model for `deploy-tui` calling `systemctl start deploy-check.service` from
  a non-root TUI session, given the unit is now confirmed to be a **system-wide** unit —
  this needs an actual answer now (e.g. polkit rule, sudo, or running the TUI with
  elevated privileges), it's no longer a hypothetical from the original design doc.
- `internal/history/history.go` has been written **and successfully tested against real
  journal data** on the dev machine, via a throwaway diagnostic `main.go` in
  `cmd/deploy-tui` (fetches entries, prints grouped-by-run output). Confirmed: entry
  counts match expected stage counts per run, grouping by `DEPLOY_RUN_ID` correctly
  separates distinct runs, and duration values parse as expected. This test surfaced the
  `SYSLOG_IDENTIFIER` bug described above, which has since been fixed and re-verified.
  **Next immediate step:** replace the throwaway diagnostic `main.go` with the real
  bubbletea model, wiring `history.FetchEntries`/`GroupByRun` in as a `tea.Cmd` for
  async loading.
- The user is switching to **Claude Code** (terminal-based) to continue implementation
  work from this point forward, specifically for the bubbletea wiring, since it
  benefits from direct file access and the ability to run `go build`/`go test` and see
  real compiler errors during iteration. These handoff docs are intended to be read by
  a new Claude Code session to restore full context.

## How to use these docs in a new session

Paste `00-overview.md` plus the relevant `0X-*.md` file for the component being
discussed. This overview file is the single source of truth for what's actually been
built and verified versus what's still planned — prefer it over the original component
docs' framing where they might conflict, since those were written before implementation
began.
