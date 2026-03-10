package registry

import "time"

// Skill describes a capability an agent exposes.
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

// Agent is the registry's representation of a registered agent.
type Agent struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	URL           string            `json:"url"`
	CardURL       string            `json:"card_url,omitempty"`
	Skills        []Skill           `json:"skills"`
	Status        string            `json:"status"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
	RegisteredAt  time.Time         `json:"registered_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// registerRequest is the POST /api/agents body.
type registerRequest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	URL         string            `json:"url"`
	CardURL     string            `json:"card_url,omitempty"`
	Skills      []Skill           `json:"skills"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Token       string            `json:"token,omitempty"`
}

// registerResponse is returned by POST /api/agents.
type registerResponse struct {
	ID    string `json:"id"`
	Token string `json:"token,omitempty"`
}

// Identity mirrors the workspace/identity.json format used by picobot agents.
type Identity struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url,omitempty"`
	CardURL     string   `json:"card_url,omitempty"`
	Skills      []Skill  `json:"skills,omitempty"`
}
