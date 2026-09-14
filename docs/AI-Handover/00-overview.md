# Deploy-Manager — Project Handoff / Overview (Updated)

This document supersedes the earlier `00-overview.md`. It reflects actual implementation
progress, not just the original plan. **All six components are built and verified,
including a real production install on the actual CircleMud game server** — components
1–4 (skeleton, gitutil/pipeline, `deploy-check`, systemd units) were done first;
component 5 (`deploy-tui`) now has a working list view, drill-down log view, trigger-now,
and no-op visibility/filtering; component 6 (packaging/install scripts) took the project
from "build manually on the target" to "build a tarball on the dev machine, copy it,
install with one script." Paste this file into any new session before continuing work,
along with the component-specific doc for whatever you're working on.

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
  **Second fix, found while building the `deploy-tui` log drill-down:**
  `journal.Send` (go-systemd v22.7.0) silently drops a field's value entirely — it comes
  back as JSON `null` via `journalctl -o json`, no error returned — once that field's
  serialized size crosses a page-size-shaped cliff around **~4096 bytes**. Verified
  empirically by bisection: a 4000-byte field value came through fine, 4200 bytes came
  through `null`. This was silently eating the real `configure`/`make` command output
  (`DEPLOY_OUTPUT`, added for the log drill-down — see field list below) on every real
  build, since that output routinely runs several KB. Fixed by capping `DEPLOY_OUTPUT` at
  3500 bytes, keeping the *tail* (that's where a failing command's actual error shows
  up) — see `journallog.truncateOutput`. No known way to raise this ceiling; if another
  field ever needs to carry more data, shrink it rather than trying to find a "correct"
  larger size.
- **Scheduling: systemd timer, no resident daemon.** `deploy-check` is a one-shot binary
  invoked by a `.timer` unit; it runs and exits. **Installed and verified as a
  system-wide unit** (`/etc/systemd/system/`, not a user unit) — chosen because the tool
  should keep working regardless of whether anyone is logged in interactively.
- **TUI: bubbletea + lipgloss.** Fully wired to real data now: a newest-first list of
  deploy runs, a drill-down view (`enter`/`esc`) showing per-stage output, a "trigger
  now" action (`t`) that calls the real systemd unit, and a no-op visibility toggle
  (`n`). See component 5 doc for the full detail.
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
| 5 | `deploy-tui` | Go, bubbletea + lipgloss | **Done (MVP), installed on the real game server.** See `05-deploy-tui.md`. |
| 6 | Packaging / install scripts | bash (`build_package.sh`, `install.sh`) | **Done, used for the real game-server install.** See `06-packaging-deploy.md`. |

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
      history.go        <- component 5, fetches + parses journal entries
  resources/             <- component 6, templates packaged by build_package.sh
    config.json
    deploy-check.service   (User=__SERVICE_USER__ placeholder, templated by install.sh)
    deploy-check.timer
  scripts/               <- component 6
    build_package.sh     <- dev machine only (needs the Go SDK); builds static
                             binaries + tars up a self-contained install package
    install.sh            <- runs on the target machine from inside the extracted
                             package; no Go needed there
  dist/                  <- build_package.sh output (gitignored), one .tar.gz per build
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
  **A no-op check (nothing new to deploy) now also gets its own `DEPLOY_RUN_ID`** — see
  the `noop` status below; this was originally not the case (see "Open questions"'s
  history for why it changed).
- `DEPLOY_STAGE` — one of: `backup`, `pull`, `configure`, `make`, or `check` (the single
  synthetic stage logged for a no-op run — see `DEPLOY_STATUS: noop` below)
- `DEPLOY_STATUS` — one of: `success`, `failure`, or `noop` (a no-op check: remote SHA
  already matches `last-deployed-sha`, nothing was deployed). `deploy-tui` shows a
  `noop`-only run as `NO-OP` in the STATUS column, in the terminal's default color
  (neither the PASS-green nor FAIL-red used for real stage outcomes).
- `DEPLOY_SHA` — the commit SHA being deployed (or, for a `noop` run, the remote SHA that
  was checked and found to already be deployed)
- `DEPLOY_DURATION_MS` — stage duration in milliseconds, as a string (all journal field
  values are strings; convert with `strconv` where a numeric value is needed). Always
  `"0"` for a `noop` run's `check` stage — the SHA comparison itself isn't timed.
- `DEPLOY_OUTPUT` — the stage's captured command output (`pipeline.StageResult.Output`:
  `git pull`/`./configure`/`make`'s combined stdout+stderr), added specifically for
  `deploy-tui`'s log drill-down (`enter` on a run). **Capped at 3500 bytes, keeping the
  tail** — see the `journal.Send` size-cliff bug described above; this is a hard ceiling,
  not a stylistic choice. Empty string for a `pull` success (that stage's
  `StageResult.Output` is only populated on error — see `02-gitutil-pipeline.md`) and for
  a `noop` run's `check` stage.
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
6. If `remoteSHA == lastSHA` and not `--force`: log a single `DEPLOY_STAGE=check`,
   `DEPLOY_STATUS=noop` journal entry (its own fresh `DEPLOY_RUN_ID`), print "no new
   commits" to stdout, and exit 0 cleanly. **This used to write no journal entry at all**
   (see `05-deploy-tui.md`'s history for why that changed — a no-op `deploy-tui` trigger
   was otherwise indistinguishable from a silently broken one).
7. If `--dry-run`: print what would be deployed and exit, without running anything (no
   journal entry either — dry-run is a manual diagnostic action, not a real check).
8. Otherwise, generate a `runID` (UUID) and run stages in order via `runPipeline`:
   `backup` (only if `cfg.EnableBackup`) → `pull` → `configure` → `make`. Each stage
   result is logged via
   `journallog.LogStage(runID, stage, status, sha, durationMs, output)`.
   Stops at the first failing stage (does not attempt later stages or any cleanup).
9. On full success: atomically overwrite `state_file_path` with the new SHA, print
   success, exit 0. On any stage failure: exit 1, `last-deployed-sha` remains unchanged.

## systemd units (as installed)

Installed system-wide (chosen deliberately over user units, since the tool should run
regardless of interactive login state):

- `/etc/systemd/system/deploy-check.service` — `Type=oneshot`, runs as a non-root user
  (`User=` templated to the installing account — see below), `SyslogIdentifier=deploy-check`,
  `Wants=network-online.target` / `After=network-online.target` (added because
  `deploy-check` needs network access for git operations, and a boot-time run could
  otherwise race against networking not being up yet).
- `/etc/systemd/system/deploy-check.timer` — `OnBootSec=2min`, `OnUnitActiveSec=15min`,
  `Persistent=true` (a missed run due to downtime fires once at next boot).
- Binary installed to `/usr/local/bin/deploy-check`; config at
  `/etc/deployment-manager/config.json` (note: **not** `/etc/deploy-manager/` — an
  earlier path used in this doc's original draft and still the CLI flag's hardcoded
  default; the real installed path uses the full `deployment-manager` name to match the
  Go module/repo name, and every actual install always passes `--config` explicitly, so
  the flag default is effectively dead and safe to ignore).
- **No longer installed by hand.** As of component 6, both units ship as templates in
  `resources/` (`deploy-check.service` has `User=__SERVICE_USER__` as a placeholder) and
  `scripts/install.sh` substitutes the real service account, copies both files into
  place, runs `daemon-reload`, and does `systemctl enable --now deploy-check.timer` — see
  `06-packaging-deploy.md`.
- Verified working end-to-end, both on the dev machine and the real game server:
  `sudo systemctl start deploy-check.service` triggers a real run, visible via
  `journalctl -u deploy-check` / `journalctl -t deploy-check`.

## Open questions / not yet decided (updated)

Resolved since the last update:
- ~~No-op runs don't produce a journal entry~~ — **resolved.** They now log a
  `DEPLOY_STAGE=check`/`DEPLOY_STATUS=noop` entry with its own `DEPLOY_RUN_ID`. This both
  helps confirm the timer is firing on schedule and (the actual trigger for fixing it)
  makes `deploy-tui`'s "trigger now" give honest feedback instead of looking broken when
  nothing was actually wrong — see `05-deploy-tui.md`.
- ~~Permission model for `deploy-tui` triggering `deploy-check.service`~~ — **resolved.**
  A scoped, passwordless sudoers rule (`/etc/sudoers.d/deploy-tui`, limited to exactly
  `systemctl start deploy-check.service`), installed automatically by `install.sh`. See
  `05-deploy-tui.md` and `06-packaging-deploy.md`.

Still carried over, still open:
- **Backup retention/pruning** — explicitly out of scope for now. The person
  configuring the app being able to choose whether backups run at all is **already
  implemented** via `EnableBackup` in the config. Actual pruning of old backup folders
  is still unimplemented and deliberately deferred.
- Locking: current behavior on lock failure is silent exit 1 with an stderr message, no
  journal entry — whether that should also log to the journal is still open.
- Whether a failed stage should attempt any rollback/cleanup — currently does not; state
  is left as-is for manual inspection.
- Missing parent directories for `backup_dir` or `state_file_path` currently cause a hard
  failure (`cp`/`os.WriteFile` don't create missing parents) rather than being
  auto-created — flagged as a small hardening item, not yet done.
- `pipeline.RunPull`'s `StageResult.Output` is only populated on error, never on success
  — so a passing `pull` stage always shows "(no output captured)" in `deploy-tui`'s log
  drill-down. Known, minor, not yet fixed.

New, specific to `deploy-tui` (see `05-deploy-tui.md` for full detail) — none of these
block anything, they're just not built:
- Whether the TUI should support live-tailing a triggered run (`journalctl -f`) versus
  only showing history after the run completes — still just showing history; not
  decided either way.
- Whether the TUI should ever expose config editing, versus staying strictly
  read-only + trigger-only — current lean is still "read-only + trigger," not settled
  formally, but nothing has pushed on this since.

## How to use these docs in a new session

Paste `00-overview.md` plus the relevant `0X-*.md` file for the component being
discussed. This overview file is the single source of truth for what's actually been
built and verified versus what's still planned — prefer it over the original component
docs' framing where they might conflict, since those were written before implementation
began. There are now six component docs: `01`–`04` (skeleton, gitutil/pipeline,
`deploy-check`, systemd units — all done, stable, rarely need revisiting), `05`
(`deploy-tui`), and `06` (`build_package.sh` / `install.sh` — read this one before
touching anything about how the app gets onto a machine).

## Status as of the last session (for a quick "what's next" read)

Everything described in this document is built, tested, and **running on the real
CircleMud game server**, installed via the `build_package.sh` → copy tarball →
`install.sh` flow. The immediate next steps, if any come up, are more likely to be small
refinements (e.g. fixing `pull`'s missing success output, deciding on live-tailing) than
new components — there's no large piece of unbuilt scope left from the original plan.
