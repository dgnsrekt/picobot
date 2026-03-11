package registry

import "github.com/a2aproject/a2a-go/a2a"

// registerRequest is the POST /api/agents body.
type registerRequest struct {
	URL      string            `json:"url"`
	Card     *a2a.AgentCard    `json:"card,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Token    string            `json:"token,omitempty"`
}

// registerResponse is returned by POST /api/agents.
type registerResponse struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}
