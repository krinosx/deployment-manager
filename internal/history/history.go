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
