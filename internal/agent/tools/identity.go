package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

// IdentitySkill represents a single capability the agent exposes.
type IdentitySkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// IdentityData holds the agent's public identity fields.
type IdentityData struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	URL         string          `json:"url"`
	Skills      []IdentitySkill `json:"skills"`
}

// UpdateIdentityTool reads or updates identity.json in the workspace.
// No in-memory cache — every call reads from and writes to disk directly.
type UpdateIdentityTool struct {
	workspace string
}

func NewUpdateIdentityTool(workspace string) *UpdateIdentityTool {
	return &UpdateIdentityTool{workspace: workspace}
}

func (t *UpdateIdentityTool) Name() string { return "update_identity" }

func (t *UpdateIdentityTool) Description() string {
	return "Read or update this agent's identity (name, description, URL, skills). Call with no arguments to read current identity. Partial updates are merged into existing values. Changes take effect immediately."
}

func (t *UpdateIdentityTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Agent display name",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Agent description",
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "External A2A URL for this agent",
			},
			"skills": map[string]interface{}{
				"type":        "array",
				"description": "Full replacement of the skills list",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":          map[string]interface{}{"type": "string"},
						"name":        map[string]interface{}{"type": "string"},
						"description": map[string]interface{}{"type": "string"},
						"tags": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "string"},
						},
					},
					"required": []string{"id", "name"},
				},
			},
		},
	}
}

func (t *UpdateIdentityTool) Execute(_ context.Context, args map[string]interface{}) (string, error) {
	path := filepath.Join(t.workspace, "identity.json")

	// Read current identity from disk (empty struct if file absent).
	var data IdentityData
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &data)
	}

	// Merge provided fields.
	updated := false
	if v, ok := args["name"].(string); ok {
		data.Name = v
		updated = true
	}
	if v, ok := args["description"].(string); ok {
		data.Description = v
		updated = true
	}
	if v, ok := args["url"].(string); ok {
		data.URL = v
		updated = true
	}
	if raw, ok := args["skills"]; ok {
		// Re-marshal then unmarshal to convert []interface{} → []IdentitySkill.
		b, err := json.Marshal(raw)
		if err == nil {
			var skills []IdentitySkill
			if err := json.Unmarshal(b, &skills); err == nil {
				data.Skills = skills
				updated = true
			}
		}
	}

	// Write back only when something changed.
	if updated {
		b, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return "", err
		}
	}

	// Always return the current state.
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
