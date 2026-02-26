package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPTool wraps a single MCP tool and implements tools.Tool so it can be
// registered in picobot's tool registry alongside built-in tools.
type MCPTool struct {
	serverName string
	def        *mcp.Tool
	session    *mcp.ClientSession
}

// Name returns a namespaced tool name: mcp__<server>__<tool>.
func (t *MCPTool) Name() string {
	return fmt.Sprintf("mcp__%s__%s", t.serverName, t.def.Name)
}

// Description returns the MCP tool's description.
func (t *MCPTool) Description() string {
	return t.def.Description
}

// Parameters converts the MCP InputSchema (map[string]any from the SDK) into
// the map[string]interface{} expected by picobot's tool registry.
func (t *MCPTool) Parameters() map[string]interface{} {
	if t.def.InputSchema == nil {
		return nil
	}

	// InputSchema arrives from the SDK as map[string]any; go through JSON
	// round-trip to get a clean map[string]interface{}.
	b, err := json.Marshal(t.def.InputSchema)
	if err != nil {
		return nil
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(b, &schema); err != nil {
		return nil
	}
	return schema
}

// Execute calls the remote MCP tool and returns concatenated text content.
func (t *MCPTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	result, err := t.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      t.def.Name,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		var msgs []string
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				msgs = append(msgs, tc.Text)
			}
		}
		return "", fmt.Errorf("mcp tool error: %s", strings.Join(msgs, "; "))
	}

	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}
