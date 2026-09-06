# Component 5: `deploy-tui` (Updated — In Progress)

See `00-overview.md` for full context, current architecture status, and the
`DEPLOY_RUN_ID` addition before continuing work here. **This is the actively in-progress
component.** Everything below reflects real progress made so far, not just the original
plan.

## Progress so far

1. **Dependencies added**: `github.com/charmbracelet/bubbletea`,
   `github.com/charmbracelet/lipgloss`.
2. **Minimal bubbletea skeleton built and verified working** in `cmd/deploy-tui/main.go`
   — renders a placeholder message, quits cleanly on `q`/`ctrl+c`. This confirmed the
   Model-Update-View wiring compiles and runs correctly before any real data was
   introduced. (The actual "hello world" code is simple enough to recreate from
   scratch if needed — see the bubbletea docs' basic example, or ask for it again; not
   worth preserving verbatim here since it will be replaced by the real model next.)
3. **`internal/history/history.go` written** (fetches and parses real journal data) —
   **not yet tested against real data**. This is the immediate next step when work
   resumes.

## `internal/history/history.go` (as written, untested)

```go
package history

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Entry represents a single journal line for one pipeline stage.
type Entry struct {
	RunID      string `json:"DEPLOY_RUN_ID"`
	Stage      string `json:"DEPLOY_STAGE"`
	Status     string `json:"DEPLOY_STATUS"`
	SHA        string `json:"DEPLOY_SHA"`
	DurationMs string `json:"DEPLOY_DURATION_MS"`
	Timestamp  string `json:"__REALTIME_TIMESTAMP"`
	Message    string `json:"MESSAGE"`
}

// FetchEntries runs `journalctl` for the deploy-check unit and parses each
// line as a JSON object.
func FetchEntries() ([]Entry, error) {
	cmd := exec.Command("journalctl", "-t", "deploy-check", "-o", "json", "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl failed: %w", err)
	}
	return parseEntries(output)
}

// parseEntries decodes journalctl's JSON-lines output (one JSON object per
// line, not a JSON array) into a slice of Entry.
func parseEntries(output []byte) ([]Entry, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))

	var entries []Entry
	for decoder.More() {
		var e Entry
		if err := decoder.Decode(&e); err != nil {
			return nil, fmt.Errorf("failed to parse journal entry: %w", err)
		}
		if e.RunID != "" {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// Run represents all stages belonging to one deploy-check execution.
type Run struct {
	RunID  string
	SHA    string
	Stages []Entry
}

// GroupByRun groups a flat list of entries into runs, preserving the order
// runs first appear in.
func GroupByRun(entries []Entry) []Run {
	var runs []Run
	index := make(map[string]int)

	for _, e := range entries {
		i, exists := index[e.RunID]
		if !exists {
			runs = append(runs, Run{RunID: e.RunID, SHA: e.SHA})
			i = len(runs) - 1
			index[e.RunID] = i
		}
		runs[i].Stages = append(runs[i].Stages, e)
	}
	return runs
}
```

Implementation notes:
- Filters on `-t deploy-check`, matching `SyslogIdentifier=deploy-check` from the
  systemd unit (component 4) — not the process/binary name, which would break under
  `go run` vs. the installed binary.
- `journalctl -o json` emits **JSON Lines** (one object per line), not a JSON array —
  hence the `json.Decoder` + `decoder.More()` loop rather than a single `Unmarshal`
  call.
- All journal field values arrive as strings, including `DEPLOY_DURATION_MS` — no
  numeric conversion happens in this package yet; that's expected to happen at display
  time (via `strconv.ParseInt`) wherever the TUI actually renders duration.
- `GroupByRun` relies entirely on `DEPLOY_RUN_ID` (see `00-overview.md`) — this is
  exactly why that field was added before writing this function, avoiding a
  SHA+timestamp heuristic.
- The `e.RunID != ""` filter guards against any journal entries matching the `-t`
  filter that aren't actually from this tool (unlikely given the specific tag, but a
  cheap safety check).

## Immediate next step when resuming

Test `FetchEntries()` + `GroupByRun()` against real journal data on the dev machine —
e.g. via a throwaway `main.go` or a quick test — **before** wiring this into the
bubbletea `View()`. Confirms the parsing logic works against actual `journalctl` output
on this specific machine/systemd version before building UI on top of it.

## Remaining scope (not yet started)

- Wire `history.FetchEntries`/`GroupByRun` into the bubbletea model as a `tea.Cmd` (async
  load, so the UI doesn't block while `journalctl` runs) — need to introduce a custom
  `tea.Msg` type (e.g. `historyLoadedMsg`) fed back into `Update`.
- List view: one line per `Run` (SHA, derived overall pass/fail from its `Stages`,
  total/last-stage timestamp).
- Drill-down view: per-stage breakdown for a selected run, showing captured output
  (`Message` field) — useful for inspecting a failure.
- "Trigger now" action — **blocked on an unresolved permission question**: since
  `deploy-check.service` is a system-wide unit (see `04-systemd-units.md`), calling
  `systemctl start deploy-check.service` from a regular-user TUI session needs a
  privilege-escalation mechanism (sudo prompt, polkit rule, or running the TUI itself
  with elevated rights). Not yet decided — needs resolving before this feature can be
  built, not just a nice-to-have.
- Styling via lipgloss — not started, currently plain-text rendering only.

## Open questions specific to this component

- How to actually invoke `systemctl start` with appropriate permissions from the TUI
  (see above — the single biggest open item for this component right now).
- Whether the TUI should support live-tailing a triggered run (`journalctl -f`) versus
  only showing completed history — original design allowed for either; not decided.
- Whether the TUI should remain strictly read-only + trigger-only, or ever expose any
  config editing — current lean is still "read-only + trigger," not settled formally.
