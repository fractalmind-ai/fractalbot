package runtime

import (
	"context"
	"fmt"
	"strings"
)

// ToolCommandPlanner turns tool-prefixed messages into tool calls.
type ToolCommandPlanner struct{}

// NextStep decides the next tool call or reply.
func (p *ToolCommandPlanner) NextStep(ctx context.Context, req PlannerRequest) (PlannerResponse, error) {
	_ = ctx
	if req.LastToolResult != nil {
		if strings.TrimSpace(req.LastToolResult.Err) != "" {
			return PlannerResponse{Reply: fmt.Sprintf("❌ %s", req.LastToolResult.Err)}, nil
		}
		return PlannerResponse{Reply: strings.TrimSpace(req.LastToolResult.Output)}, nil
	}

	name, args, ok := parseToolInvocation(req.Task.Text)
	if !ok || name == "" {
		return PlannerResponse{Reply: "usage: tool <name> <args...> (see tools.list)"}, nil
	}
	return PlannerResponse{ToolCall: &ToolCall{Name: name, Args: args}}, nil
}
