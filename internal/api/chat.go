package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
)

// chatAgent routes a chat message through an agent's MCP-backed skills and
// returns the joined textual output. Each skill maps to a single tool on an
// MCP server; the caller supplies the user input and the agent's system prompt
// as tool arguments.
func (s *Server) chatAgent(ctx context.Context, tenantID, agentID, content string) (string, error) {
	agent, err := s.Store.GetAgent(ctx, tenantID, agentID)
	if err != nil {
		return "", fmt.Errorf("get agent %q: %w", agentID, err)
	}
	if len(agent.Skills) == 0 {
		return "", fmt.Errorf("agent has no skills")
	}

	outputs := make([]string, 0, len(agent.Skills))
	for _, skill := range agent.Skills {
		server, err := s.Store.GetMcpServer(ctx, tenantID, skill.ServerID)
		if err != nil {
			return "", fmt.Errorf("get mcp server %q for skill %q: %w", skill.ServerID, skill.Name, err)
		}
		transport, err := mcp.TransportFromServer(server.Transport)
		if err != nil {
			return "", fmt.Errorf("parse transport for mcp server %q: %w", server.ID, err)
		}
		res, err := s.MCP.CallTool(ctx, transport, skill.Tool, map[string]any{
			"input":  content,
			"prompt": agent.SystemPrompt,
		})
		if err != nil {
			return "", fmt.Errorf("call tool %q on mcp server %q: %w", skill.Tool, server.ID, err)
		}
		outputs = append(outputs, mcp.CallToolResult(res))
	}

	return strings.Join(outputs, "\n"), nil
}
