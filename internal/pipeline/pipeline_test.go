package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

// setupFakeProject creates a throwaway project directory with a minimal
// ./configure script and Makefile, so pipeline stages can be tested without
// a real CircleMud checkout.
func setupFakeProject(t *testing.T) Config {
	t.Helper()

	repoDir := t.TempDir()
	srcDir := filepath.Join(repoDir, "src")
	libDir := filepath.Join(repoDir, "lib")
	backupDir := t.TempDir()

	if err := os.Mkdir(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	if err := os.Mkdir(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}
	// Give lib/ some content so backup has something real to copy.
	if err := os.WriteFile(filepath.Join(libDir, "world.dat"), []byte("fake data"), 0644); err != nil {
		t.Fatalf("failed to write fake lib file: %v", err)
	}

	// Fake ./configure — just needs to exist, be executable, and exit 0.
	configureScript := "#!/bin/sh\necho configuring...\nexit 0\n"
	configurePath := filepath.Join(repoDir, "configure")
	if err := os.WriteFile(configurePath, []byte(configureScript), 0755); err != nil {
		t.Fatalf("failed to write fake configure script: %v", err)
	}

	// Fake Makefile with "clean" and default targets, mirroring real usage
	// (make clean, then make -j4).
	makefile := "all:\n\techo building...\n\nclean:\n\techo cleaning...\n"
	if err := os.WriteFile(filepath.Join(srcDir, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatalf("failed to write fake Makefile: %v", err)
	}

	return Config{
		RepoDir:   repoDir,
		SrcDir:    srcDir,
		LibDir:    libDir,
		BackupDir: backupDir,
	}
}

func TestRunConfigure(t *testing.T) {
	cfg := setupFakeProject(t)

	result := RunConfigure(cfg)

	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Output)
	}
}

func TestRunMake(t *testing.T) {
	cfg := setupFakeProject(t)

	result := RunMake(cfg)

	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Output)
	}
}

func TestRunBackup(t *testing.T) {
	cfg := setupFakeProject(t)

	result := RunBackup(cfg)

	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Output)
	}

	entries, err := os.ReadDir(cfg.BackupDir)
	if err != nil {
		t.Fatalf("failed to read backup dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 backup folder, found %d", len(entries))
	}

	backedUpFile := filepath.Join(cfg.BackupDir, entries[0].Name(), "world.dat")
	if _, err := os.Stat(backedUpFile); err != nil {
		t.Errorf("expected backed-up file to exist at %s: %v", backedUpFile, err)
	}
}
