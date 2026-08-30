package journallog

import (
	"fmt"
	"time"
)

// LogStage records the outcome of a single pipeline stage. This stdout
// implementation is a stand-in for real systemd journal logging, to be
// swapped in later without changing call sites.
func LogStage(stage string, status string, sha string, durationMs int64) {
	fmt.Printf(
		"[%s] stage=%s status=%s sha=%s duration_ms=%d\n",
		time.Now().Format(time.RFC3339),
		stage,
		status,
		sha,
		durationMs,
	)
}
