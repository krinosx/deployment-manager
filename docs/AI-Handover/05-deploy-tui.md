# Component 5: `deploy-tui`

See `00-overview.md` for full context, architecture table, and cross-component
contracts (journal field names) before making decisions here.

## Scope

A terminal UI, built last since it's a pure view over data structures/contracts that
components 1–4 already define. Two main capabilities:

1. **Browse run history** — read from the systemd journal, not a custom store.
2. **Trigger a check manually** — either run `deploy-check` directly or trigger it via
   systemd, then show/stream progress.

## Stack

- Go, `github.com/charmbracelet/bubbletea` for the TUI framework/event loop,
  `github.com/charmbracelet/lipgloss` for styling/layout.

## History view

- Data source: `journalctl -u deploy-check -o json --no-pager` (exact filter — by unit
  name vs. `SyslogIdentifier=deploy-check` via `-t deploy-check` — should match whatever
  component 4 settles on), invoked via `os/exec` and parsed as JSONL (one JSON object
  per line).
- Each parsed entry exposes the standardized fields from the overview:
  `DEPLOY_STAGE`, `DEPLOY_STATUS`, `DEPLOY_SHA`, `DEPLOY_DURATION_MS`, plus journald's
  own fields (`__REALTIME_TIMESTAMP`, `MESSAGE`, `PRIORITY`, etc).
- Since logging is per-stage (not per-run), the TUI needs to **group consecutive stage
  entries into a "run"** for a sensible list view (e.g. group by `DEPLOY_SHA` + rough
  timestamp proximity, since there's no explicit run/session ID in the current design —
  worth revisiting if grouping proves awkward, see open questions).
- List view: one line per run (SHA, timestamp, overall pass/fail derived from its
  stages, total duration). Drill-down view: per-stage breakdown with captured output
  (`MESSAGE` field) for a selected run.

## Manual trigger

- Two possible approaches, not yet decided between (see component 4's open questions,
  which this depends on):
  - Shell out to `systemctl start deploy-check.service`, then tail
    `journalctl -u deploy-check -f -o json` to show live progress in the TUI as new
    stage entries arrive.
  - Or invoke the `deploy-check` binary directly as a subprocess and stream its stdout —
    simpler, but bypasses the systemd-managed lock/scheduling context (still safe since
    `deploy-check` itself holds its own `flock`, per component 3).

## Open questions for this session

- How to group per-stage journal entries into logical "runs" for the list view, given
  there's no explicit run/session identifier in the current logging design. Simplest
  fix: have `deploy-check` (component 3) generate a random/incrementing run ID at start
  and include it as an additional journal field — worth raising back with component 3
  if grouping by SHA+timestamp proves unreliable (e.g. two consecutive no-op checks
  with the same SHA).
- Permission model for `systemctl start deploy-check.service` from a non-root TUI session
  — depends on component 4's decision on system vs. user unit.
- Read-only vs. read-write scope of the TUI beyond triggering — e.g. should it ever be
  able to edit the branch being tracked, or is that purely a config/systemd-unit concern
  outside the TUI's responsibility? Leaning towards TUI staying read-only + trigger-only,
  not a config editor, but not explicitly settled.
