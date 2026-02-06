package channels

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var agentNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)

var errDefaultAgentMissing = errors.New("default agent is not configured")
var noAgentsConfiguredMessage = "⚠️ No agents configured.\nSet agents.ohMyCode.defaultAgent or agents.ohMyCode.allowedAgents."

// AgentSelection describes the resolved target agent and task text.
type AgentSelection struct {
	Agent     string
	Task      string
	Specified bool
}

// AgentAllowlist enforces allowed agent names.
type AgentAllowlist struct {
	configured bool
	allowed    map[string]struct{}
}

// NewAgentAllowlist builds an allowlist from configured names.
func NewAgentAllowlist(names []string) AgentAllowlist {
	allowed := make(map[string]struct{})
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	return AgentAllowlist{configured: len(allowed) > 0, allowed: allowed}
}

// ParseAgentSelection extracts a target agent and task from chat text.
// Supported syntax: /agent <name> <task...> or /to <name> <task...>
func ParseAgentSelection(text string) (AgentSelection, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return AgentSelection{Task: ""}, nil
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return AgentSelection{Task: ""}, nil
	}

	first := fields[0]
	if strings.HasPrefix(first, "/agent") || strings.HasPrefix(first, "/to") {
		command := first
		if idx := strings.IndexByte(command, '@'); idx != -1 {
			command = command[:idx]
		}
		if command != "/agent" && command != "/to" {
			return AgentSelection{Task: trimmed}, nil
		}
		if len(fields) < 3 {
			return AgentSelection{}, fmt.Errorf("usage: /agent <name> <task>")
		}
		return AgentSelection{
			Agent:     fields[1],
			Task:      strings.Join(fields[2:], " "),
			Specified: true,
		}, nil
	}

	return AgentSelection{Task: trimmed}, nil
}

// ResolveAgentSelection applies defaults and allowlist validation.
func ResolveAgentSelection(selection AgentSelection, defaultAgent string, allowlist AgentAllowlist) (AgentSelection, error) {
	agent := strings.TrimSpace(selection.Agent)
	if !selection.Specified {
		agent = strings.TrimSpace(defaultAgent)
		if agent == "" {
			return AgentSelection{}, errDefaultAgentMissing
		}
	}

	if err := ValidateAgentName(agent); err != nil {
		return AgentSelection{}, err
	}

	if err := allowlist.Validate(agent, defaultAgent); err != nil {
		return AgentSelection{}, err
	}

	selection.Agent = agent
	return selection, nil
}

// ValidateAgentName enforces allowed agent naming.
func ValidateAgentName(name string) error {
	if !agentNamePattern.MatchString(name) {
		return fmt.Errorf("invalid agent name %q", name)
	}
	return nil
}

// Validate ensures the agent is allowed per configuration.
func (a AgentAllowlist) Validate(agentName, defaultAgent string) error {
	if a.configured {
		if _, ok := a.allowed[agentName]; !ok {
			return fmt.Errorf("agent %q is not allowed", agentName)
		}
		return nil
	}

	defaultName := strings.TrimSpace(defaultAgent)
	if defaultName == "" {
		return errDefaultAgentMissing
	}
	if agentName != defaultName {
		return fmt.Errorf("agent %q is not allowed", agentName)
	}
	return nil
}

// Names returns the configured allowlist names in sorted order.
func (a AgentAllowlist) Names() []string {
	if !a.configured {
		return nil
	}
	names := make([]string, 0, len(a.allowed))
	for name := range a.allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func filterOutAgentName(names []string, target string) []string {
	if target == "" {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if name == target {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

func agentNotAllowedMessage(err error, defaultAgent string, allowlist AgentAllowlist) string {
	message := fmt.Sprintf("❌ %v", err)
	if allowlist.configured {
		return fmt.Sprintf("%s\nTip: add to agents.ohMyCode.allowedAgents or use /agents.", message)
	}
	if strings.TrimSpace(defaultAgent) != "" {
		return fmt.Sprintf("%s\nOnly the default agent is enabled.\nTip: configure agents.ohMyCode.allowedAgents to allow others, or use /agents.", message)
	}
	return fmt.Sprintf("%s\nTip: configure agents.ohMyCode.defaultAgent or agents.ohMyCode.allowedAgents.", message)
}
