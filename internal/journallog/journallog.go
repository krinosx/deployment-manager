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

// LogStage records the outcome of a single pipeline stage to the systemd
// journal, tagged with structured fields for later querying via journalctl.
func LogStage(runID string, stage string, status string, sha string, durationMs int64) {
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
	}

	// Errors from journal.Send are deliberately ignored: a logging failure
	// should never abort an otherwise-successful (or already-failing) deploy.
	_ = journal.Send(message, priority, fields)
}
