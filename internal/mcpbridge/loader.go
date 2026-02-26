package mcpbridge

import (
	"context"
	"log"
	"os/exec"

	"github.com/local/picobot/internal/agent/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LoadTools connects to every MCP server in cfg, lists their tools, and
// returns picobot Tool wrappers for all of them. Failures per server are
// logged as warnings and skipped so startup isn't aborted.
//
// The returned cleanup func closes all sessions; call it (via defer) when the
// process exits.
func LoadTools(ctx context.Context, cfg *MCPConfig) ([]tools.Tool, func(), error) {
	if cfg == nil || len(cfg.Servers) == 0 {
		return nil, func() {}, nil
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "picobot", Version: "shadow"}, nil)

	var allTools []tools.Tool
	var sessions []*mcp.ClientSession

	for name, srv := range cfg.Servers {
		cmd := exec.Command(srv.Command, srv.Args...) //nolint:gosec
		for k, v := range srv.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}

		transport := &mcp.CommandTransport{Command: cmd}
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			log.Printf("mcp: failed to connect to server %q: %v", name, err)
			continue
		}

		result, err := session.ListTools(ctx, nil)
		if err != nil {
			log.Printf("mcp: failed to list tools from %q: %v", name, err)
			_ = session.Close()
			continue
		}

		sessions = append(sessions, session)
		for _, def := range result.Tools {
			allTools = append(allTools, &MCPTool{
				serverName: name,
				def:        def,
				session:    session,
			})
		}
		log.Printf("mcp: loaded %d tools from server %q", len(result.Tools), name)
	}

	cleanup := func() {
		for _, s := range sessions {
			_ = s.Close()
		}
	}
	return allTools, cleanup, nil
}
