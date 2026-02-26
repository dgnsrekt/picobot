package a2abridge

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/local/picobot/internal/agent/tools"
)

// A2ATool wraps a single A2A agent and implements tools.Tool so it can be
// registered in picobot's tool registry alongside built-in tools.
type A2ATool struct {
	agentName string
	client    *a2aclient.Client
}

// Name returns a namespaced tool name: a2a__<agent>__delegate.
func (t *A2ATool) Name() string {
	return fmt.Sprintf("a2a__%s__delegate", t.agentName)
}

// Description returns a human-readable description of the tool.
func (t *A2ATool) Description() string {
	return fmt.Sprintf("Delegate a task or question to the %s agent via A2A protocol.", t.agentName)
}

// Parameters returns the JSON Schema for tool arguments.
func (t *A2ATool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task or question to delegate to the agent.",
			},
		},
		"required": []string{"task"},
	}
}

// Execute sends the task to the remote A2A agent and returns its response.
func (t *A2ATool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	task, _ := args["task"].(string)
	if task == "" {
		return "", fmt.Errorf("task argument is required")
	}

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: task})
	result, err := t.client.SendMessage(ctx, &a2a.MessageSendParams{Message: msg})
	if err != nil {
		return "", fmt.Errorf("a2a delegate to %s failed: %w", t.agentName, err)
	}
	return extractTextFromSendResult(result), nil
}

// extractTextFromSendResult extracts human-readable text from a SendMessageResult.
func extractTextFromSendResult(result a2a.SendMessageResult) string {
	switch v := result.(type) {
	case *a2a.Message:
		return extractPartsText(v.Parts)
	case *a2a.Task:
		if v.Status.Message != nil {
			if text := extractPartsText(v.Status.Message.Parts); text != "" {
				return text
			}
		}
		for _, artifact := range v.Artifacts {
			if text := extractPartsText(artifact.Parts); text != "" {
				return text
			}
		}
		return fmt.Sprintf("task %s: %s", v.ID, v.Status.State)
	}
	return ""
}

// extractPartsText concatenates text from all TextPart values in a ContentParts slice.
func extractPartsText(parts a2a.ContentParts) string {
	var sb strings.Builder
	for _, part := range parts {
		if tp, ok := part.(a2a.TextPart); ok {
			sb.WriteString(tp.Text)
		}
	}
	return sb.String()
}

// LoadTools creates one A2ATool per configured agent. Each tool holds a client
// created once at startup. Per-agent failures are logged and skipped. Returns a
// cleanup func that destroys all clients.
func LoadTools(cfg *A2AConfig) ([]tools.Tool, func(), error) {
	if cfg == nil || len(cfg.Agents) == 0 {
		return nil, func() {}, nil
	}

	ctx := context.Background()
	var allTools []tools.Tool
	var clients []*a2aclient.Client

	for name, agentCfg := range cfg.Agents {
		client, err := a2aclient.NewFromEndpoints(ctx, []a2a.AgentInterface{
			{Transport: a2a.TransportProtocolJSONRPC, URL: agentCfg.URL},
		})
		if err != nil {
			log.Printf("a2a: failed to create client for agent %q: %v", name, err)
			continue
		}
		clients = append(clients, client)
		allTools = append(allTools, &A2ATool{agentName: name, client: client})
		log.Printf("a2a: created delegation tool for agent %q at %s", name, agentCfg.URL)
	}

	cleanup := func() {
		for _, c := range clients {
			_ = c.Destroy()
		}
	}
	return allTools, cleanup, nil
}
