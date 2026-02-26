package a2abridge

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// A2AAgentConfig describes a remote A2A agent to delegate tasks to.
type A2AAgentConfig struct {
	URL string `json:"url"`
}

// A2AConfig is the top-level structure of ~/.picobot/a2a.json.
type A2AConfig struct {
	Agents map[string]A2AAgentConfig `json:"agents"`
	Serve  bool                      `json:"serve"`
	Port   int                       `json:"port"` // default 8080
}

// LoadA2AConfig reads <cfgDir>/a2a.json. Returns (nil, nil) if the file is
// absent so callers can skip A2A setup without treating absence as an error.
func LoadA2AConfig(cfgDir string) (*A2AConfig, error) {
	path := filepath.Join(cfgDir, "a2a.json")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg A2AConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
