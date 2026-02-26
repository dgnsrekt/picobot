package mcpbridge

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// MCPServerConfig describes one MCP server entry in mcp.json.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// MCPConfig is the top-level structure of ~/.picobot/mcp.json.
type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"mcpServers"`
}

// LoadMCPConfig reads <cfgDir>/mcp.json. Returns (nil, nil) if the file is
// absent so callers can skip MCP setup without treating absence as an error.
func LoadMCPConfig(cfgDir string) (*MCPConfig, error) {
	path := filepath.Join(cfgDir, "mcp.json")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg MCPConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
