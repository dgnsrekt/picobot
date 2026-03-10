package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Client talks to the a2agent-registry HTTP API.
type Client struct {
	baseURL   string
	agentID   string
	token     string
	tokenPath string
	http      *http.Client
}

// NewClient creates a registry client. tokenDir is the directory where the
// registration token is persisted (typically ~/.picobot/).
func NewClient(baseURL, agentID, tokenDir string) *Client {
	return &Client{
		baseURL:   baseURL,
		agentID:   agentID,
		tokenPath: filepath.Join(tokenDir, "registry-token"),
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

// Register sends the agent's identity to the registry. On first registration
// a token is returned and persisted; on subsequent calls the saved token is
// sent so the registry recognises the agent.
func (c *Client) Register(ctx context.Context, id Identity, metadata map[string]string) error {
	c.loadToken()

	body := registerRequest{
		ID:          c.agentID,
		Name:        id.Name,
		Description: id.Description,
		URL:         id.URL,
		CardURL:     id.CardURL,
		Skills:      id.Skills,
		Metadata:    metadata,
		Token:       c.token,
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
		return fmt.Errorf("registry: register returned HTTP %d", resp.StatusCode)
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
		fmt.Sprintf("%s/api/agents/%s/heartbeat", c.baseURL, c.agentID),
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
		fmt.Sprintf("%s/api/agents/%s", c.baseURL, c.agentID),
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

// ReadIdentity loads identity.json from the given workspace directory.
func ReadIdentity(workspace string) (Identity, error) {
	data, err := os.ReadFile(filepath.Join(workspace, "identity.json"))
	if err != nil {
		return Identity{}, err
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return Identity{}, fmt.Errorf("registry: parse identity.json: %w", err)
	}
	return id, nil
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
