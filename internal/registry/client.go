package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
)

// Client talks to the a2agent-registry HTTP API.
type Client struct {
	baseURL   string
	agentURL  string
	token     string
	tokenPath string
	http      *http.Client
}

// NewClient creates a registry client. agentURL is the agent's reachable URL
// (used as the registry key). tokenDir is the directory where the registration
// token is persisted (typically ~/.picobot/).
func NewClient(baseURL, agentURL, tokenDir string) *Client {
	return &Client{
		baseURL:   baseURL,
		agentURL:  agentURL,
		tokenPath: filepath.Join(tokenDir, "registry-token"),
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

// Register sends the agent card to the registry. On first registration a token
// is returned and persisted; on subsequent calls the saved token is sent so the
// registry recognises the agent.
func (c *Client) Register(ctx context.Context, card *a2a.AgentCard, metadata map[string]string) error {
	c.loadToken()

	body := registerRequest{
		URL:      c.agentURL,
		Card:     card,
		Metadata: metadata,
		Token:    c.token,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("registry: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/agents", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("registry: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry: register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registry: register returned HTTP %d: %s", resp.StatusCode, body)
	}

	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("registry: decode: %w", err)
	}

	if result.Token != "" {
		c.token = result.Token
		c.saveToken()
	}

	return nil
}

// Heartbeat sends a keep-alive to the registry.
func (c *Client) Heartbeat(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/agents/%s/heartbeat", c.baseURL, url.PathEscape(c.agentURL)),
		http.NoBody)
	if err != nil {
		return fmt.Errorf("registry: heartbeat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry: heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("registry: heartbeat returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Deregister removes the agent from the registry.
func (c *Client) Deregister(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/api/agents/%s", c.baseURL, url.PathEscape(c.agentURL)),
		http.NoBody)
	if err != nil {
		return fmt.Errorf("registry: deregister request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry: deregister: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry: deregister returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// StartHeartbeat runs a background goroutine that sends heartbeats at the
// given interval until the context is cancelled.
func (c *Client) StartHeartbeat(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Heartbeat(ctx); err != nil {
					log.Printf("registry: heartbeat failed: %v", err)
				}
			}
		}
	}()
	log.Printf("registry: heartbeat started (every %s)", interval)
}

// identityJSON mirrors the workspace/identity.json format used by picobot agents.
type identityJSON struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url,omitempty"`
	Skills      []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags,omitempty"`
	} `json:"skills,omitempty"`
}

// ReadIdentity loads identity.json from the given workspace directory and
// returns it as an a2a.AgentCard.
func ReadIdentity(workspace string) (*a2a.AgentCard, error) {
	data, err := os.ReadFile(filepath.Join(workspace, "identity.json"))
	if err != nil {
		return nil, err
	}
	var id identityJSON
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("registry: parse identity.json: %w", err)
	}

	card := &a2a.AgentCard{
		Name:               id.Name,
		Description:        id.Description,
		URL:                id.URL,
		Version:            "1.0.0",
		ProtocolVersion:    "0.3.0",
		PreferredTransport: a2a.TransportProtocolJSONRPC,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}

	for _, s := range id.Skills {
		card.Skills = append(card.Skills, a2a.AgentSkill{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
		})
	}

	return card, nil
}

func (c *Client) loadToken() {
	data, err := os.ReadFile(c.tokenPath)
	if err == nil && len(data) > 0 {
		c.token = string(data)
	}
}

func (c *Client) saveToken() {
	if err := os.WriteFile(c.tokenPath, []byte(c.token), 0600); err != nil {
		log.Printf("registry: failed to save token: %v", err)
	}
}
