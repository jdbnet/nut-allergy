package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type agentConfig struct {
	Server          string     `json:"server"`
	Hostname        string     `json:"hostname"`
	CertFile        string     `json:"cert_file"`
	KeyFile         string     `json:"key_file"`
	UPSIDs          []string   `json:"ups_ids"`
	TimeoutOverride *int       `json:"timeout_override_seconds"`
	OnBattery       bool       `json:"on_battery"`
	Deadline        *time.Time `json:"deadline"`
}

func loadConfig(path string) (agentConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return agentConfig{}, err
	}
	var cfg agentConfig
	err = json.Unmarshal(b, &cfg)
	return cfg, err
}

func saveConfig(path string, cfg agentConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func defaultPaths(dir string) (cert, key string) {
	return filepath.Join(dir, "agent.crt"), filepath.Join(dir, "agent.key")
}
