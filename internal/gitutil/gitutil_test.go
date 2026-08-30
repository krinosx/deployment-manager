package gitutil

import (
	"os/exec"
	"testing"
)

// setupTestRepos creates a bare "origin" repo and a working clone pointing
// at it, so tests can exercise real git commands without network access.
// Returns the working repo's directory.
func setupTestRepos(t *testing.T) string {
	t.Helper()

	originDir := t.TempDir()
	workDir := t.TempDir()

	run := func(dir string, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("setup command %v in %s failed: %v\n%s", args, dir, err, out)
		}
		return string(out)
	}

	// Bare repo acting as "origin".
	run(originDir, "init", "--bare", "-b", "main")

	// Working clone with a commit, pushed up to origin.
	run(workDir, "init", "-b", "main")
	run(workDir, "config", "user.email", "test@example.com")
	run(workDir, "config", "user.name", "Test")
	run(workDir, "commit", "--allow-empty", "-m", "init")
	run(workDir, "remote", "add", "origin", originDir)
	run(workDir, "push", "origin", "main")

	return workDir
}

func TestRemoteHeadSHA(t *testing.T) {
	repoDir := setupTestRepos(t)

	sha, err := RemoteHeadSHA(repoDir, "main")
	if err != nil {
		t.Fatalf("RemoteHeadSHA returned error: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected a 40-char SHA, got %q (%d chars)", sha, len(sha))
	}
}

func TestRemoteHeadSHA_MissingBranch(t *testing.T) {
	repoDir := setupTestRepos(t)

	_, err := RemoteHeadSHA(repoDir, "does-not-exist")
	if err == nil {
		t.Fatal("expected an error for a missing branch, got nil")
	}
}

func TestPullBranch(t *testing.T) {
	repoDir := setupTestRepos(t)

	// Nothing new to pull yet, but this confirms PullBranch runs cleanly
	// against a valid, up-to-date repo.
	if err := PullBranch(repoDir, "main"); err != nil {
		t.Fatalf("PullBranch returned error: %v", err)
	}
}
