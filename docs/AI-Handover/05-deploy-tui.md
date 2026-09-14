# Component 5: `deploy-tui` (Updated — Done, MVP, installed on the game server)

See `00-overview.md` for full context, current architecture status, and the two
`go-systemd` gotchas (`SYSLOG_IDENTIFIER`, and the ~4096-byte field size cliff) before
continuing work here. **This component now has a working, real-data TUI**: a list of
deploy runs, a per-run log drill-down, a trigger-now action, and no-op run
visibility/filtering. It's been built, verified with real journal data on the dev
machine, and reinstalled onto the actual CircleMud game server via component 6's
packaging flow.

## Feature summary (all implemented and verified)

- **List view**: every `deploy-check` run — real deploys *and* no-op checks — as one row,
  newest-first: short SHA, STATUS, timestamp. Selected row highlighted (background).
- **Drill-down** (`enter` to open, `esc` to go back): per-stage breakdown of the selected
  run — stage name, status, duration, and the stage's captured command output
  (`DEPLOY_OUTPUT`, truncated to 3500 bytes server-side — see `00-overview.md`).
- **Trigger now** (`t`): runs `sudo systemctl start deploy-check.service` (passwordless,
  via the sudoers rule `install.sh` sets up — see below), waits for it to finish (a
  oneshot unit's `systemctl start` blocks until the process exits), then refreshes the
  list and reports explicitly whether a new run appeared or the check found nothing new.
- **No-op runs are now visible**: `deploy-check` logs a `DEPLOY_STAGE=check`/
  `DEPLOY_STATUS=noop` entry for every check that finds nothing new to deploy (see
  `03-deploy-check.md`), and `deploy-tui` shows these as `NO-OP` in the STATUS column, in
  the terminal's **default color** (no green/red, unlike PASS/FAIL).
- **No-op filter** (`n`): toggles whether NO-OP rows are shown. Defaults to **shown**
  (`showNoOp: true`) — the whole point of logging them was visibility, so hiding is the
  opt-in, not the default.
- **Refresh** (`r`): manual re-fetch, independent of triggering.
- `q` / `ctrl+c`: quit, from either view.

## Why no-op runs got their own status and filter (the actual story)

Originally, a no-op check produced no journal entry at all — see `03-deploy-check.md`.
This meant pressing `t` in `deploy-tui` against an up-to-date repo (the common case: the
timer runs every 15 minutes, and most checks find nothing new) did something real
(`systemctl start` genuinely ran and exited 0) but produced **zero visible feedback** —
indistinguishable from the trigger silently doing nothing. This was reported as "trigger
looks broken." The fix went through two stages:

1. First pass: compare the top run's ID before and after a triggered refresh, and show
   an explicit status message either way ("a new run was recorded" vs. "found no new
   commits"). This made the *trigger* honest but didn't change what the list itself
   showed.
2. Second pass (this session's actual ask): make **every** `deploy-check` execution —
   not just triggered ones, the periodic timer runs too — show up as a row, so the list
   itself is a true activity log, not just a deploy log. This is the `NO-OP` status +
   `n` filter described above. `journallog.LogStage` already supported arbitrary
   stage/status strings, so this needed no protocol change — just `deploy-check` calling
   it once for the no-op path (`stage="check"`, `status="noop"`), and
   `history.Run.OverallStatus()` recognizing `"noop"` as its own category ahead of the
   PASS/FAIL check.

With the timer running every 15 minutes, expect the list to accumulate a `NO-OP` row
roughly every 15 minutes it finds nothing new — this is intentional, and `n` exists
specifically to declutter that when you just want to see real deploys.

## `internal/history/history.go` (current shape)

```go
package history

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Entry represents a single journal line for one pipeline stage.
type Entry struct {
	RunID      string `json:"DEPLOY_RUN_ID"`
	Stage      string `json:"DEPLOY_STAGE"`
	Status     string `json:"DEPLOY_STATUS"`
	SHA        string `json:"DEPLOY_SHA"`
	DurationMs string `json:"DEPLOY_DURATION_MS"`
	Output     string `json:"DEPLOY_OUTPUT"`
	Timestamp  string `json:"__REALTIME_TIMESTAMP"`
	Message    string `json:"MESSAGE"`
}

func FetchEntries() ([]Entry, error) { /* journalctl -t deploy-check -o json */ }
func parseEntries(output []byte) ([]Entry, error) { /* JSON-lines decode, filters e.RunID != "" */ }

// Run represents all stages belonging to one deploy-check execution — including a
// no-op check, which has exactly one stage ("check", status "noop").
type Run struct {
	RunID  string
	SHA    string
	Stages []Entry
}

func GroupByRun(entries []Entry) []Run { /* groups by RunID, preserves first-seen order (oldest-first) */ }

// OverallStatus: NO-OP takes priority over the PASS/FAIL check, since a no-op run's
// single stage has Status "noop", not "success"/"failure".
func (r Run) OverallStatus() string {
	if len(r.Stages) == 0 {
		return "UNKNOWN"
	}
	for _, s := range r.Stages {
		if s.Status == "noop" {
			return "NO-OP"
		}
	}
	for _, s := range r.Stages {
		if s.Status != "success" {
			return "FAIL"
		}
	}
	return "PASS"
}

// Timestamp: parsed from the last stage's __REALTIME_TIMESTAMP (microsecond epoch).
func (r Run) Timestamp() time.Time { /* time.UnixMicro(parsed last-stage timestamp) */ }
```

Implementation notes (unchanged from the original, still accurate):
- Filters on `-t deploy-check` (matches `SyslogIdentifier=deploy-check`), not the
  process/binary name — works whether run via `go run`, a `/tmp` test build, or the
  installed binary.
- `journalctl -o json` is JSON Lines, not a JSON array — hence `json.Decoder` +
  `decoder.More()`.
- All journal field values are strings; `DurationMs` isn't converted here, only at
  display time.
- `GroupByRun` relies entirely on `DEPLOY_RUN_ID`, which is exactly why that field exists
  (see `00-overview.md`) — no-op runs get their own fresh ID too, so they group correctly
  as single-stage runs rather than accidentally merging with anything.

## `cmd/deploy-tui/main.go` architecture (current)

Real bubbletea `model`, not the throwaway diagnostic described in earlier versions of
this doc (that diagnostic is long gone — replaced in the first pass of this work).

- **Model fields**: `runs []history.Run`, `cursor int`, `loading bool`, `err error`,
  `mode viewMode` (`viewList` / `viewDetail`), `triggering bool`, `statusMsg string`,
  `showNoOp bool` (defaults `true`), plus `pendingTriggerRefresh bool` and
  `preTriggerTopRunID string` (bookkeeping for the "did triggering actually change
  anything" comparison described above).
- **`visibleRuns()`** is the one place that filters `runs` by `showNoOp`. The cursor and
  *all* rendering/indexing (list rows, `enter`'s target, `renderDetail`) go through this,
  never `m.runs` directly, so what's on screen and what "enter" opens always agree. The
  one deliberate exception: the pre/post-trigger comparison (`preTriggerTopRunID`) always
  uses the **unfiltered** `m.runs`, because whether a trigger did something shouldn't
  depend on whether NO-OP rows happen to be hidden at the moment.
- **`tea.Cmd`s**: `loadHistory` (async `journalctl` fetch + `GroupByRun`, reversed to
  newest-first — `GroupByRun` itself returns oldest-first) and `triggerDeploy` (runs
  `sudo systemctl start deploy-check.service`, blocks until the oneshot unit finishes,
  returns `triggerDoneMsg{output, err}`).
- **Key bindings** (list view): `↑`/`k`, `↓`/`j` navigate; `enter` opens drill-down; `n`
  toggles no-op visibility; `t` triggers; `r` refreshes; `q`/`ctrl+c` quit. Detail view:
  `esc` back to list, `q`/`ctrl+c` quit.
- **Styling**: `passStyle` (green+bold) for `PASS`/`success`, `failStyle` (red+bold) for
  `FAIL`/`failure`, everything else (including `NO-OP`/`noop` and `UNKNOWN`) falls
  through to a plain `lipgloss.NewStyle()` — i.e. the terminal's default color. This is
  *why* NO-OP needed no new styling work: it was already the default case in every status
  switch. The selected row gets a background highlight (`selectedStyle`) applied to the
  whole plain-text line — deliberately never nesting one `lipgloss.Style.Render()` call's
  output inside another, since that causes the inner call's implicit trailing reset code
  to also kill the outer style partway through the line (a real bug hit and fixed during
  this work — sibling/concatenated `Render()` calls are fine, nested ones are not).

## Verification methodology (for anyone extending this)

`deploy-tui` needs a real TTY (bubbletea takes over the terminal), so it can't be
launched directly via a plain subprocess call. Verified interactively in real terminal
sessions throughout, and additionally scripted via a Python `pty.openpty()` harness for
automated checks in this session: spawn the binary attached to a pty, feed it key bytes
(`b"\r"` for enter, `b"\x1b"` for escape, `b"t"`/`b"n"`/`b"r"`/`b"q"`, etc.) with `select`
to poll for output, then read back the ANSI-rendered screen contents to confirm the
correct rows/colors/messages appeared. This is how the trigger flow, drill-down, and
no-op filter were each confirmed against the real installed binary and real journal data,
not just by reading the code.

## Verified against real usage

- Full interactive session against real journal data on the dev machine: list renders
  correctly, cursor navigation and `enter`/`esc` work, drill-down shows real `configure`/
  `make` build output (after fixing the go-systemd size-cliff bug — see
  `00-overview.md`).
- `t` confirmed to really invoke `systemctl start deploy-check.service` passwordlessly
  (`sudo -n systemctl start ...` succeeds with no prompt) and to correctly report both
  outcomes: a genuine new run appearing, and a no-op check finding nothing new.
- NO-OP rows and the `n` toggle confirmed against the real installed `deploy-check`
  binary and real systemd timer-triggered runs.
- **Reinstalled and verified on the actual production game server**, via component 6's
  `build_package.sh` → copy → `install.sh` flow — this is the first real non-dev-machine
  deployment of the whole project.

## Remaining scope (not started, none of it blocking)

- **Styling refinements** beyond the current pass/fail/plain coloring — not started,
  functional but minimal.
- **Live-tailing** a triggered run (`journalctl -f`-style streaming) instead of only
  showing history after the fact — still just fire-and-refresh, as originally decided.
- **Config editing** from within the TUI — still deliberately out of scope; the lean is
  still "read-only + trigger," unchanged from earlier.
- Detail-view scrolling for very long stage output — currently just prints the full
  (already-truncated-server-side) output inline; hasn't been a problem in practice since
  3500 bytes rarely overflows one screen, but a genuinely huge terminal history could get
  long. Not addressed.

## Open questions

- Whether live-tailing is worth building — no signal either way, not decided.
- Whether config editing should ever be in scope, or stays a hard "no" — leaning no,
  not formalized.

Resolved (kept here for history — see `00-overview.md`'s "Open questions" for the
authoritative current list):
- How `deploy-tui` gets permission to trigger a system-wide unit — a scoped, passwordless
  sudoers rule, installed automatically by `install.sh` (component 6). This was the
  single biggest open item for this component; it's fully resolved and working, on both
  the dev machine and the game server.
