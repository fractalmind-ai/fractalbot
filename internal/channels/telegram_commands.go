package channels

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultMonitorLines = 60
	maxMonitorLines     = 200
)

func parseMonitorArgs(parts []string) (string, int, error) {
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", 0, fmt.Errorf("usage: /monitor <name> [lines]")
	}
	if len(parts) > 3 {
		return "", 0, fmt.Errorf("usage: /monitor <name> [lines]")
	}

	agentName := strings.TrimSpace(parts[1])
	lines := defaultMonitorLines
	if len(parts) == 3 {
		parsed, err := parseMonitorLines(parts[2])
		if err != nil {
			return "", 0, err
		}
		lines = parsed
	}
	if lines > maxMonitorLines {
		lines = maxMonitorLines
	}
	return agentName, lines, nil
}

func parseMonitorLines(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultMonitorLines, nil
	}
	lines, err := strconv.Atoi(value)
	if err != nil || lines <= 0 {
		return 0, fmt.Errorf("invalid lines value %q", raw)
	}
	return lines, nil
}

func validateAgentCommandName(agentName, defaultAgent string, allowlist AgentAllowlist) error {
	trimmed := strings.TrimSpace(agentName)
	if trimmed == "" {
		return fmt.Errorf("agent name is required")
	}
	if err := ValidateAgentName(trimmed); err != nil {
		return err
	}
	if err := allowlist.Validate(trimmed, defaultAgent); err != nil {
		return err
	}
	return nil
}
