package journallog

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/coreos/go-systemd/v22/journal"
)

func TestLogStage(t *testing.T) {
	if !journal.Enabled() {
		t.Skip("systemd journal not available on this system")
	}

	LogStage("make", "success", "abc123", 4200)

	// journalctl needs a moment to index; a short real-world check would
	// normally poll, but for a quick manual sanity check this is enough.
	out, err := exec.Command("journalctl", "--user", "-n", "20", "--no-pager").CombinedOutput()
	if err != nil {
		t.Skipf("could not query journalctl: %v", err)
	}
	if !strings.Contains(string(out), "deploy stage make: success") {
		t.Error("expected recent journal entry not found")
	}
}
