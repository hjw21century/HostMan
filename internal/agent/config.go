package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds agent configuration.
type Config struct {
	Server      string `json:"server"`
	APIKey      string `json:"api_key"`
	IntervalSec int    `json:"interval_sec"`
	NoServices  bool   `json:"no_services"`
	LogFile     string `json:"log_file"`
}

const defaultConfigPath = "/etc/hostman/agent.json"

// LoadConfig reads config from a JSON file.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Server == "" {
		return nil, fmt.Errorf("config: server is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("config: api_key is required")
	}
	if cfg.IntervalSec <= 0 {
		cfg.IntervalSec = 60
	}
	return &cfg, nil
}

// SaveConfig writes a config file (used by install script).
func SaveConfig(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
