package gitutil

import (
	"fmt"
	"os/exec"
	"strings"
)

// RemoteHeadSHA returns the commit SHA that `branch` currently points to on
// the "origin" remote, without modifying the local working tree.
func RemoteHeadSHA(repoDir, branch string) (string, error) {
	cmd := exec.Command("git", "ls-remote", "origin", branch)
	cmd.Dir = repoDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote failed: %w", err)
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return "", fmt.Errorf("branch %q not found on remote", branch)
	}

	fields := strings.Fields(line)
	if len(fields) < 1 {
		return "", fmt.Errorf("unexpected git ls-remote output: %q", line)
	}

	return fields[0], nil
}

func PullBranch(repoDir, branch string) error {

	cmd := exec.Command("git", "pull", "--ff-only", "origin", branch)
	cmd.Dir = repoDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w \n output: %s", err, output)
	}

	return nil
}
