package journallog

import (
	"fmt"
	"strconv"
	"time"

	"github.com/coreos/go-systemd/v22/journal"
)

func LogStageDebug(stage string, status string, sha string, durationMs int64) {
	fmt.Printf(
		"[%s] stage=%s status=%s sha=%s duration_ms=%d\n",
		time.Now().Format(time.RFC3339),
		stage,
		status,
		sha,
		durationMs,
	)
}

// maxOutputBytes bounds how much of a stage's captured command output gets
// written to the journal per entry. This is capped well below 4KB, not just
// for storage hygiene: github.com/coreos/go-systemd/v22/journal.Send (as of
// v22.7.0) silently drops a field's value entirely (it comes back as JSON
// null via journalctl, no error returned) once that field's serialized size
// crosses a page-size-shaped cliff around ~4096 bytes. Verified empirically:
// a 4000-byte DEPLOY_OUTPUT came through fine, 4200 bytes came through null.
const maxOutputBytes = 3500

// LogStage records the outcome of a single pipeline stage to the systemd
// journal, tagged with structured fields for later querying via journalctl.
func LogStage(runID string, stage string, status string, sha string, durationMs int64, output string) {
	priority := journal.PriInfo
	if status == "failure" {
		priority = journal.PriErr
	}

	message := "deploy stage " + stage + ": " + status

	fields := map[string]string{
		"SYSLOG_IDENTIFIER":  "deploy-check",
		"DEPLOY_RUN_ID":      runID,
		"DEPLOY_STAGE":       stage,
		"DEPLOY_STATUS":      status,
		"DEPLOY_SHA":         sha,
		"DEPLOY_DURATION_MS": strconv.FormatInt(durationMs, 10),
		"DEPLOY_OUTPUT":      truncateOutput(output),
	}

	// Errors from journal.Send are deliberately ignored: a logging failure
	// should never abort an otherwise-successful (or already-failing) deploy.
	_ = journal.Send(message, priority, fields)
}

// truncateOutput keeps the tail of s, since the most useful diagnostic
// content (the actual error) is usually at the end of a failed command's
// output.
func truncateOutput(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	return "...(truncated)...\n" + s[len(s)-maxOutputBytes:]
}
