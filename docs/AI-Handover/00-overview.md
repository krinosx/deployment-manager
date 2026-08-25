# Deploy-Manager — Project Handoff / Overview

This document is a handoff for continuing design/implementation discussions in new
sessions. It captures the full context and decisions made so far. Companion files
(`01-...md` through `05-...md`) go into detail on each component and are meant to be
used in focused sessions about that specific piece — but each links back here so an
agent always has the full picture before making a decision.

## Background / Problem

- The user runs a **CircleMud** (a MUD/game server, C codebase) on a resource-constrained
  Ubuntu Linux server. Two game instances run concurrently. RAM is tight — headroom exists
  for the game itself, but not for extra memory-hungry tooling running alongside it.
- The game's code is versioned in Git. Current manual deploy process:
  1. Back up the `lib/` folder (game data).
  2. `git pull` latest code for a specific branch.
  3. Run `./configure` in the project root.
  4. `cd src && make clean && make -j4` — produces a new `circle` binary in `/bin`.
  5. An admin manually logs into the running game and issues an in-game command to
     restart a given instance (this step is **intentionally not automated** — see
     "Explicit non-goals" below).

## Goal

Build a small Linux-native utility ("deploy-manager") that:
- Periodically checks a specific branch of the git repo for new commits.
- When a new commit is found, runs the build pipeline (backup → pull → configure → make)
  automatically.
- Logs each run (and each stage within a run) for later inspection.
- Offers a TUI to trigger a check manually and browse run history/logs.

## Key constraints & decisions

- **RAM is precious.** No resident daemon holding memory 24/7 if avoidable.
- **No portability requirement.** This is Linux-only, native, and can lean fully into
  systemd/journald rather than being cross-platform-safe.
- **Game restart stays manual.** An admin restarts each instance in-game because tests
  may be running against a live instance at deploy time. The deploy tool must NOT restart
  the game. This is safe because on Linux, overwriting/replacing a running binary's file
  on disk does not affect the currently-executing process (the OS keeps the old inode
  open via the process's existing file descriptor). So `deploy-check` can rebuild and
  atomically swap the `circle` binary at any time without disturbing a currently running
  or under-test instance. The new binary is picked up next time an admin manually restarts.

## Chosen tech stack

- **Language: Go**, for both the checker and the TUI.
  - Idle/short-lived Go binaries have a small memory footprint (single static binary,
    no interpreter runtime), which fits the RAM constraint well.
  - `os/exec` is used to shell out to the real `git`, `./configure`, and `make` —
    no embedded git library needed.
- **TUI: [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss)**.
- **Logging: systemd journal**, via `github.com/coreos/go-systemd/v22/journal`, using
  structured key=value fields (not just plain text). No custom log store/format.
- **Scheduling: systemd timer**, not a resident daemon. `deploy-check` is a one-shot
  binary invoked periodically by a `.timer` unit; it does its work and exits. Idle
  memory cost is effectively zero between runs since systemd itself is already resident.
- **State: minimal.** The only persistent value needed outside the journal is
  "last deployed SHA" — a single small file (e.g. `/var/lib/deploy-manager/last-deployed-sha`),
  written atomically after a successful deploy. Everything else (run history, stage
  output) lives in the journal and is queried via `journalctl`, not re-implemented.

## Architecture summary

| # | Component | Stack | Detail doc |
|---|---|---|---|
| 1 | Project skeleton (Go module layout) | Go module | `01-project-skeleton.md` |
| 2 | `gitutil` + `pipeline` packages (backup, pull, configure, make — each a discrete stage) | Go, `os/exec` | `02-gitutil-pipeline.md` |
| 3 | `deploy-check` binary (one-shot: check remote SHA vs last-deployed, run pipeline if different, log per-stage journal entries, update last-deployed-sha on success) | Go + `go-systemd/journal` | `03-deploy-check.md` |
| 4 | systemd `.service` (oneshot) + `.timer` units | systemd unit files | `04-systemd-units.md` |
| 5 | `deploy-tui` (history browser via `journalctl -o json`, manual trigger via `systemctl start`) | Go, bubbletea + lipgloss | `05-deploy-tui.md` |

Recommended build order matches the table above — each component depends on the ones
before it. Component 2 is the core logic and should be testable standalone (via a
throwaway `main.go` or tests) before wiring it into `deploy-check`.

## Standardized journal fields (locked in at component 1, used by 3 and 5)

These field names are a cross-component contract — component 3 writes them,
component 5 parses them, so they should not be changed without updating both.

- `DEPLOY_STAGE` — one of: `backup`, `pull`, `configure`, `make`
- `DEPLOY_STATUS` — one of: `start`, `success`, `failure`
- `DEPLOY_SHA` — the commit SHA being deployed (short or full — decide in component 1)
- `DEPLOY_DURATION_MS` — stage duration in milliseconds

One journal entry is written per pipeline stage (not one entry per whole run), so a
failed `make` is immediately visible and queryable on its own, e.g.:
```bash
journalctl -u deploy-check -o json | jq 'select(.DEPLOY_STAGE=="make" and .DEPLOY_STATUS=="failure")'
```

## Open questions / not yet decided

These are explicitly left open for the component-specific sessions to resolve:

- Exact locking mechanism (`flock` path, held for the duration of a full pipeline run)
  to prevent a manual TUI-triggered run and a timer-triggered run from racing.
- Whether a check-only run (no new commit found) still logs a journal entry, and if so
  at what verbosity/priority.
- Behavior on partial failure: if `configure` or `make` fails mid-pipeline, should
  `last-deployed-sha` remain unchanged (implied "yes" so far, not yet formalized), and
  should the tool attempt any cleanup of a partially-built tree?
- Whether to keep a backup/rollback copy of the previous working `circle` binary before
  overwriting it (discussed as a nice-to-have, not committed to yet).
- Backup retention/pruning policy for the `lib/` backups (currently just "keep doing
  what's done manually today").
- Exact CLI flags for `deploy-check` (e.g. `--dry-run`, `--force`) — mentioned as useful
  for standalone testing in component 3, not yet specified.

## How to use these docs in a new session

Paste `00-overview.md` plus the relevant `0X-*.md` file for the component being
discussed. The overview gives full context and cross-component contracts (journal
field names, state file, non-goals); the component doc gives the specific scope,
responsibilities, and open questions for that piece.
