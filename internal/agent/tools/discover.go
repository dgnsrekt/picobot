package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DiscoverAgentsTool queries the agent registry to find other agents by skill,
// name, or tag. Returns agent names, descriptions, skills, URLs, and status.
type DiscoverAgentsTool struct {
	registryURL string
	client      *http.Client
}

func NewDiscoverAgentsTool(registryURL string) *DiscoverAgentsTool {
	return &DiscoverAgentsTool{
		registryURL: registryURL,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *DiscoverAgentsTool) Name() string { return "discover_agents" }
func (t *DiscoverAgentsTool) Description() string {
	return "Search the agent registry to find other agents by skill, name, or tag. Returns agent names, descriptions, skills, URLs, and status."
}

func (t *DiscoverAgentsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"skill": map[string]interface{}{
				"type":        "string",
				"description": "Search for agents with this skill (fuzzy match on skill name/id)",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Search for agents by name (substring match)",
			},
			"tag": map[string]interface{}{
				"type":        "string",
				"description": "Search for agents with skills tagged with this value",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Filter by status: online, offline (default: all)",
			},
		},
	}
}

type discoveryEntry struct {
	Card struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Version     string `json:"version"`
		Skills      []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
		} `json:"skills"`
	} `json:"card"`
	Status   string            `json:"status"`
	Metadata map[string]string `json:"metadata"`
}

func (t *DiscoverAgentsTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	params := url.Values{}
	for _, key := range []string{"skill", "name", "tag", "status"} {
		if v, ok := args[key].(string); ok && v != "" {
			params.Set(key, v)
		}
	}

	reqURL := t.registryURL + "/api/agents"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("discover_agents: request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("discover_agents: registry unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("discover_agents: registry returned HTTP %d", resp.StatusCode)
	}

	var entries []discoveryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return "", fmt.Errorf("discover_agents: decode: %w", err)
	}

	return formatDiscoveryResults(entries), nil
}

func formatDiscoveryResults(entries []discoveryEntry) string {
	if len(entries) == 0 {
		return "No agents found matching your query."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Agent Registry: found %d agent(s)\n", len(entries))

	for i, e := range entries {
		fmt.Fprintf(&sb, "\n%d. %s (%s)\n", i+1, e.Card.Name, e.Status)
		fmt.Fprintf(&sb, "   URL: %s\n", e.Card.URL)
		if e.Card.Description != "" {
			fmt.Fprintf(&sb, "   Description: %s\n", e.Card.Description)
		}
		if e.Card.Version != "" {
			fmt.Fprintf(&sb, "   Version: %s\n", e.Card.Version)
		}
		if len(e.Metadata) > 0 {
			parts := make([]string, 0, len(e.Metadata))
			for k, v := range e.Metadata {
				parts = append(parts, k+"="+v)
			}
			fmt.Fprintf(&sb, "   Metadata: %s\n", strings.Join(parts, ", "))
		}
		if len(e.Card.Skills) > 0 {
			fmt.Fprintf(&sb, "   Skills:\n")
			for _, s := range e.Card.Skills {
				line := fmt.Sprintf("   - %s: %s", s.Name, s.Description)
				if len(s.Tags) > 0 {
					line += fmt.Sprintf(" [tags: %s]", strings.Join(s.Tags, ", "))
				}
				fmt.Fprintln(&sb, line)
			}
		}
	}

	return sb.String()
}
