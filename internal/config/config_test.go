package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	cfg, err := Load("testdata/config.json")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Branch != "main" {
		t.Errorf("expected branch %q, got %q", "main", cfg.Branch)
	}
	if !cfg.EnableBackup {
		t.Error("expected EnableBackup to be true")
	}
	if cfg.Pipeline.RepoDir != "/opt/mud" {
		t.Errorf("expected repo_dir %q, got %q", "/opt/mud", cfg.Pipeline.RepoDir)
	}
	if cfg.StateFilePath != "/var/lib/deploy-manager/last-deployed-sha" {
		t.Errorf("unexpected state file path: %q", cfg.StateFilePath)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("testdata/does-not-exist.json")
	if err == nil {
		t.Fatal("expected an error for a missing config file, got nil")
	}
}
