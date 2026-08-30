package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/krinosx/deployment-manager/internal/config"
	"github.com/krinosx/deployment-manager/internal/gitutil"
	"github.com/krinosx/deployment-manager/internal/journallog"
	"github.com/krinosx/deployment-manager/internal/pipeline"
)

func main() {
	configPath := flag.String("config", "/etc/deploy-manager/config.json", "path to config file")
	dryRun := flag.Bool("dry-run", false, "check for a new commit but don't deploy")
	force := flag.Bool("force", false, "deploy even if no new commit is found")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	lockPath := cfg.StateFilePath + ".lock"
	lockFile, err := acquireLock(lockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer lockFile.Close()

	lastSHA, err := readLastSHA(cfg.StateFilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	remoteSHA, err := gitutil.RemoteHeadSHA(cfg.Pipeline.RepoDir, cfg.Branch)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if remoteSHA == lastSHA && !*force {
		fmt.Println("no new commits, nothing to do")
		return
	}

	if *dryRun {
		fmt.Printf("dry run: would deploy %s\n", remoteSHA)
		return
	}

	if !runPipeline(cfg, remoteSHA) {
		os.Exit(1)
	}

	if err := writeLastSHA(cfg.StateFilePath, remoteSHA); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Println("deploy successful:", remoteSHA)
}

// runPipeline runs each enabled stage in order, logging results, and stops
// at the first failure. Returns false if any stage failed.
func runPipeline(cfg config.Config, sha string) bool {
	type stage struct {
		name string
		run  func() pipeline.StageResult
	}

	var stages []stage

	if cfg.EnableBackup {
		stages = append(stages, stage{"backup", func() pipeline.StageResult {
			return pipeline.RunBackup(cfg.Pipeline)
		}})
	}
	stages = append(stages,
		stage{"pull", func() pipeline.StageResult {
			return pipeline.RunPull(cfg.Pipeline, cfg.Branch)
		}},
		stage{"configure", func() pipeline.StageResult {
			return pipeline.RunConfigure(cfg.Pipeline)
		}},
		stage{"make", func() pipeline.StageResult {
			return pipeline.RunMake(cfg.Pipeline)
		}},
	)

	for _, s := range stages {
		result := s.run()

		status := "success"
		if !result.Success {
			status = "failure"
		}
		journallog.LogStage(result.Stage, status, sha, result.Duration.Milliseconds())

		if !result.Success {
			fmt.Fprintf(os.Stderr, "stage %q failed: %s\n", result.Stage, result.Output)
			return false
		}
	}

	return true
}

func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another run is already in progress")
	}
	return f, nil
}

func readLastSHA(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read state file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func writeLastSHA(path, sha string) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(sha), 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}
	return nil
}
