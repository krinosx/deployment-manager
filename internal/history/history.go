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

// Run represents all stages belonging to one deploy-check execution.
type Run struct {
	RunID  string
	SHA    string
	Stages []Entry
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

// OverallStatus derives a single pass/fail result for the run from its
// stages: any non-"success" stage fails the whole run.
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

// Timestamp returns the time of the run's last stage entry, parsed from the
// journal's microsecond-epoch string. Returns the zero time if unavailable.
func (r Run) Timestamp() time.Time {
	if len(r.Stages) == 0 {
		return time.Time{}
	}
	micros, err := strconv.ParseInt(r.Stages[len(r.Stages)-1].Timestamp, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMicro(micros)
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
