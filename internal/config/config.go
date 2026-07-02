// Package config loads and validates the VLA config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the LLM connection settings loaded from config.json.
type Config struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// Load reads, parses, and validates the config file at path.
// Returns an error if the file is missing, unreadable, malformed JSON,
// or fails validation (empty api_key or model).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("config: api_key is required")
	}
	if c.Model == "" {
		return fmt.Errorf("config: model is required")
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.openai.com/v1"
	}
	return nil
}
