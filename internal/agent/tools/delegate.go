package tools

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

// DelegateTaskTool sends a task to another A2A agent and returns the response.
type DelegateTaskTool struct {
	resolver *agentcard.Resolver
}

func NewDelegateTaskTool() *DelegateTaskTool {
	return &DelegateTaskTool{
		resolver: agentcard.DefaultResolver,
	}
}

func (t *DelegateTaskTool) Name() string { return "delegate_task" }
func (t *DelegateTaskTool) Description() string {
	return "Send a task to another A2A agent and wait for its response. Use this after discover_agents to delegate work to a specific agent."
}

func (t *DelegateTaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_url": map[string]interface{}{
				"type":        "string",
				"description": "The URL of the agent to delegate to (from discover_agents results)",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The message/task to send to the agent",
			},
		},
		"required": []string{"agent_url", "message"},
	}
}

func (t *DelegateTaskTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	agentURL, _ := args["agent_url"].(string)
	message, _ := args["message"].(string)
	if agentURL == "" {
		return "", fmt.Errorf("delegate_task: agent_url is required")
	}
	if message == "" {
		return "", fmt.Errorf("delegate_task: message is required")
	}

	// Use a generous timeout for the full round-trip (card fetch + task execution)
	delegateCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	// Fetch agent card
	card, err := t.resolver.Resolve(delegateCtx, agentURL)
	if err != nil {
		return "", fmt.Errorf("delegate_task: fetch agent card from %s: %w", agentURL, err)
	}

	// Create client from card
	client, err := a2aclient.NewFromCard(delegateCtx, card)
	if err != nil {
		return "", fmt.Errorf("delegate_task: create client for %s: %w", card.Name, err)
	}
	defer func() {
		if err := client.Destroy(); err != nil {
			log.Printf("delegate_task: failed to destroy client for %s: %v", card.Name, err)
		}
	}()

	// Build and send message
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: message})
	params := &a2a.MessageSendParams{Message: msg}

	result, err := client.SendMessage(delegateCtx, params)
	if err != nil {
		return "", fmt.Errorf("delegate_task: send to %s: %w", card.Name, err)
	}

	return extractResultText(result, card.Name), nil
}

// extractResultText pulls text content from a SendMessageResult (either *a2a.Message or *a2a.Task).
func extractResultText(result a2a.SendMessageResult, agentName string) string {
	switch v := result.(type) {
	case *a2a.Message:
		return partsToText(v.Parts)
	case *a2a.Task:
		var sb strings.Builder
		fmt.Fprintf(&sb, "[%s task %s — %s]\n", agentName, v.ID, v.Status.State)
		// Check status message
		if v.Status.Message != nil {
			if text := partsToText(v.Status.Message.Parts); text != "" {
				sb.WriteString(text)
				sb.WriteString("\n")
			}
		}
		// Check artifacts
		for _, art := range v.Artifacts {
			if text := partsToText(art.Parts); text != "" {
				sb.WriteString(text)
				sb.WriteString("\n")
			}
		}
		return strings.TrimSpace(sb.String())
	default:
		return fmt.Sprintf("[%s returned unexpected result type]", agentName)
	}
}

func partsToText(parts a2a.ContentParts) string {
	var texts []string
	for _, p := range parts {
		if tp, ok := p.(a2a.TextPart); ok {
			texts = append(texts, tp.Text)
		}
	}
	return strings.Join(texts, "\n")
}
