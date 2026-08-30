package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/krinosx/deployment-manager/internal/pipeline"
)

// Config is the full application configuration, loaded from a JSON file
// at startup.
type Config struct {
	Pipeline      pipeline.Config `json:"pipeline"`
	Branch        string          `json:"branch"`
	EnableBackup  bool            `json:"enable_backup"`
	StateFilePath string          `json:"state_file_path"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	return cfg, nil
}
