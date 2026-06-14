// Package config loads the per-repo .wtdb.json file and locates the repo root.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the per-repo config file name, placed at the repo root.
const FileName = ".wtdb.json"

// Config is the user-facing .wtdb.json. Exactly three keys, by design.
type Config struct {
	// EnvFile is the dotenv-format file to copy and rewrite (e.g. ".env", ".dev.vars").
	EnvFile string `json:"envFile"`
	// DBEnvVar is the key in EnvFile whose value is a postgres://.../<dbname> URL.
	DBEnvVar string `json:"dbEnvVar"`
	// PathsToCopy are gitignored dirs/files cloned into each new worktree (e.g. "node_modules").
	PathsToCopy []string `json:"pathsToCopy"`
}

// Load reads and validates .wtdb.json from dir.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &c, nil
}

// Exists reports whether dir contains a .wtdb.json.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, FileName))
	return err == nil
}

func (c *Config) validate() error {
	if c.EnvFile == "" {
		return fmt.Errorf("envFile is required")
	}
	if c.DBEnvVar == "" {
		return fmt.Errorf("dbEnvVar is required")
	}
	return nil
}
