package pipeline

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/krinosx/deployment-manager/internal/gitutil"
)

// StageResult captures the outcome of a single pipeline stage
// (backup, pull, configure, make), for the caller to log and act on.
type StageResult struct {
	Stage    string
	Success  bool
	Output   string
	Duration time.Duration
}

// Config holds the paths and settings needed to run deploy pipeline stages.
type Config struct {
	RepoDir   string `json:"repo_dir"`
	SrcDir    string `json:"src_dir"`
	LibDir    string `json:"lib_dir"`
	BackupDir string `json:"backup_dir"`
}

func RunPull(cfg Config, branch string) StageResult {
	start := time.Now()

	err := gitutil.PullBranch(cfg.RepoDir, branch)

	result := StageResult{
		Stage:    "pull",
		Success:  err == nil,
		Duration: time.Since(start),
	}
	if err != nil {
		result.Output = err.Error()
	}

	return result
}

func RunBackup(cfg Config) StageResult {
	start := time.Now()

	timestamp := time.Now().Format("20060102-150405")
	dest := filepath.Join(cfg.BackupDir, "lib-"+timestamp)

	cmd := exec.Command("cp", "-r", cfg.LibDir, dest)
	output, err := cmd.CombinedOutput()

	result := StageResult{
		Stage:    "backup",
		Success:  err == nil,
		Duration: time.Since(start),
		Output:   string(output),
	}
	if err != nil {
		result.Output = fmt.Sprintf("cp failed: %v\n%s", err, output)
	}

	return result
}

func RunConfigure(cfg Config) StageResult {
	start := time.Now()

	cmd := exec.Command("./configure")
	cmd.Dir = cfg.RepoDir

	output, err := cmd.CombinedOutput()

	result := StageResult{
		Stage:    "configure",
		Success:  err == nil,
		Duration: time.Since(start),
		Output:   string(output),
	}
	if err != nil {
		result.Output = fmt.Sprintf("configure failed: %v\n%s", err, output)
	}

	return result
}

func RunMake(cfg Config) StageResult {
	start := time.Now()

	cleanCmd := exec.Command("make", "clean")
	cleanCmd.Dir = cfg.SrcDir
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		return StageResult{
			Stage:    "make",
			Success:  false,
			Duration: time.Since(start),
			Output:   fmt.Sprintf("make clean failed: %v\n%s", err, output),
		}
	}

	buildCmd := exec.Command("make", "-j4")
	buildCmd.Dir = cfg.SrcDir
	output, err := buildCmd.CombinedOutput()

	result := StageResult{
		Stage:    "make",
		Success:  err == nil,
		Duration: time.Since(start),
		Output:   string(output),
	}
	if err != nil {
		result.Output = fmt.Sprintf("make -j4 failed: %v\n%s", err, output)
	}

	return result
}
