package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/channels"
	"github.com/fractalmind-ai/fractalbot/internal/config"
	agentruntime "github.com/fractalmind-ai/fractalbot/internal/runtime"
	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const (
	defaultOhMyCodeAgentManagerScript = ".claude/skills/agent-manager/scripts/main.py"
	defaultOhMyCodeDefaultAgent       = "qa-1"
	defaultOhMyCodeAssignTimeout      = 90 * time.Second

	defaultOhMyCodeMonitorDelay   = 1 * time.Second
	defaultOhMyCodeMonitorTimeout = 15 * time.Second
	defaultOhMyCodeMonitorLines   = 60

	defaultOhMyCodeLifecycleTimeout = 20 * time.Second
	maxOhMyCodeMonitorLines         = 200
)

// Manager is a minimal stub for agent lifecycle management.
type Manager struct {
	config         *config.AgentsConfig
	ChannelManager *channels.Manager
	mu             sync.RWMutex
	agents         map[string]protocol.AgentInfo
	runtime        agentruntime.AgentRuntime
}

// NewManager creates a new agent manager.
func NewManager(cfg *config.AgentsConfig) *Manager {
	return &Manager{
		config: cfg,
		agents: make(map[string]protocol.AgentInfo),
	}
}

// Start initializes agent runtime.
func (m *Manager) Start(ctx context.Context) error {
	if m.runtime == nil {
		rt, err := agentruntime.NewRuntime(m.runtimeConfig(), m.memoryConfig())
		if err != nil {
			return err
		}
		m.runtime = rt
	}
	if m.runtime != nil {
		return m.runtime.Start(ctx)
	}
	return nil
}

// Stop halts agent runtime.
func (m *Manager) Stop(ctx context.Context) error {
	if m.runtime != nil {
		return m.runtime.Stop(ctx)
	}
	return nil
}

// List returns known agents.
func (m *Manager) List() []protocol.AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]protocol.AgentInfo, 0, len(m.agents))
	for _, info := range m.agents {
		agents = append(agents, info)
	}
	return agents
}

// HandleIncoming implements channels.IncomingMessageHandler.
func (m *Manager) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	if msg == nil {
		return "", nil
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		return "", nil
	}

	channel, _ := data["channel"].(string)
	if channel != "telegram" && channel != "feishu" && channel != "slack" && channel != "discord" {
		return "", nil
	}

	text, _ := data["text"].(string)
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}

	if m.runtime != nil {
		agentName, _ := data["agent"].(string)
		task := agentruntime.Task{
			Agent:    strings.TrimSpace(agentName),
			Text:     text,
			Channel:  channel,
			Metadata: runtimeMetadata(data),
		}
		out, err := m.runtime.HandleTask(ctx, task)
		if err != nil {
			return "", err
		}
		if channel == "telegram" {
			return channels.TruncateTelegramReply(out), nil
		}
		return out, nil
	}

	if m.isOhMyCodeEnabled() {
		agentName, _ := data["agent"].(string)
		out, err := m.assignOhMyCode(ctx, text, agentName)
		if err != nil {
			return "", err
		}
		if channel == "telegram" {
			return channels.TruncateTelegramReply(out), nil
		}
		return out, nil
	}

	return fmt.Sprintf("echo: %s", text), nil
}

func (m *Manager) runtimeConfig() *config.RuntimeConfig {
	if m.config == nil {
		return nil
	}
	return m.config.Runtime
}

func (m *Manager) memoryConfig() *config.MemoryConfig {
	if m.config == nil {
		return nil
	}
	return m.config.Memory
}

func runtimeMetadata(data map[string]interface{}) map[string]string {
	metadata := make(map[string]string)
	keys := []string{"chat_id", "user_id", "open_id", "channel_id", "chatType", "message"}
	for _, key := range keys {
		if value, ok := data[key]; ok {
			metadata[key] = fmt.Sprint(value)
		}
	}
	return metadata
}

func (m *Manager) isOhMyCodeEnabled() bool {
	if m.config == nil || m.config.OhMyCode == nil {
		return false
	}
	if !m.config.OhMyCode.Enabled {
		return false
	}
	return strings.TrimSpace(m.config.OhMyCode.Workspace) != ""
}

func (m *Manager) assignOhMyCode(ctx context.Context, userText, agentOverride string) (string, error) {
	workspace, script, err := m.resolveOhMyCodeWorkspaceAndScript()
	if err != nil {
		return "", err
	}

	agentName := strings.TrimSpace(agentOverride)
	if agentName == "" {
		agentName = strings.TrimSpace(m.config.OhMyCode.DefaultAgent)
		if agentName == "" {
			agentName = defaultOhMyCodeDefaultAgent
		}
	}

	timeout := defaultOhMyCodeAssignTimeout
	if m.config.OhMyCode.AssignTimeoutSeconds > 0 {
		timeout = time.Duration(m.config.OhMyCode.AssignTimeoutSeconds) * time.Second
	}

	assignCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		assignCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	assignOut, err := runOhMyCodeAgentManager(assignCtx, workspace, script, buildOhMyCodeTaskPrompt(userText), "assign", agentName)
	if err != nil {
		return "", err
	}

	if defaultOhMyCodeMonitorDelay > 0 {
		time.Sleep(defaultOhMyCodeMonitorDelay)
	}

	monitorCtx, cancel := context.WithTimeout(ctx, defaultOhMyCodeMonitorTimeout)
	defer cancel()

	monitorOut, monitorErr := runOhMyCodeAgentManager(monitorCtx, workspace, script, "", "monitor", agentName, "--lines", strconv.Itoa(defaultOhMyCodeMonitorLines))
	if monitorErr != nil {
		return assignOut, nil
	}

	snapshot := extractMonitorSnapshot(monitorOut)
	if strings.TrimSpace(snapshot) == "" {
		return assignOut, nil
	}

	return strings.TrimSpace(assignOut) + "\n\n" + snapshot, nil
}

// MonitorAgent returns the latest agent-manager monitor output.
func (m *Manager) MonitorAgent(ctx context.Context, agentName string, lines int) (string, error) {
	workspace, script, err := m.resolveOhMyCodeWorkspaceAndScript()
	if err != nil {
		return "", err
	}

	name, err := m.validateOhMyCodeAgent(agentName)
	if err != nil {
		return "", err
	}

	if lines <= 0 {
		lines = defaultOhMyCodeMonitorLines
	}
	if lines > maxOhMyCodeMonitorLines {
		lines = maxOhMyCodeMonitorLines
	}

	monitorCtx, cancel := context.WithTimeout(ctx, defaultOhMyCodeMonitorTimeout)
	defer cancel()

	return runOhMyCodeAgentManager(monitorCtx, workspace, script, "", "monitor", name, "--lines", strconv.Itoa(lines))
}

// StartAgent starts a configured agent-manager session.
func (m *Manager) StartAgent(ctx context.Context, agentName string) (string, error) {
	workspace, script, err := m.resolveOhMyCodeWorkspaceAndScript()
	if err != nil {
		return "", err
	}

	name, err := m.validateOhMyCodeAgent(agentName)
	if err != nil {
		return "", err
	}

	lifecycleCtx, cancel := context.WithTimeout(ctx, defaultOhMyCodeLifecycleTimeout)
	defer cancel()

	return runOhMyCodeAgentManager(lifecycleCtx, workspace, script, "", "start", name)
}

// StopAgent stops a running agent-manager session.
func (m *Manager) StopAgent(ctx context.Context, agentName string) (string, error) {
	workspace, script, err := m.resolveOhMyCodeWorkspaceAndScript()
	if err != nil {
		return "", err
	}

	name, err := m.validateOhMyCodeAgent(agentName)
	if err != nil {
		return "", err
	}

	lifecycleCtx, cancel := context.WithTimeout(ctx, defaultOhMyCodeLifecycleTimeout)
	defer cancel()

	return runOhMyCodeAgentManager(lifecycleCtx, workspace, script, "", "stop", name)
}

// DoctorAgentManager runs a diagnostic check for agent-manager.
func (m *Manager) Doctor(ctx context.Context) (string, error) {
	workspace, script, err := m.resolveOhMyCodeWorkspaceAndScript()
	if err != nil {
		return "", err
	}

	lifecycleCtx, cancel := context.WithTimeout(ctx, defaultOhMyCodeLifecycleTimeout)
	defer cancel()

	return runOhMyCodeAgentManager(lifecycleCtx, workspace, script, "", "doctor")
}

func (m *Manager) resolveOhMyCodeWorkspaceAndScript() (string, string, error) {
	if m.config == nil || m.config.OhMyCode == nil {
		return "", "", errors.New("agents.ohMyCode is not configured")
	}
	if !m.config.OhMyCode.Enabled {
		return "", "", errors.New("agents.ohMyCode is disabled")
	}

	workspace := strings.TrimSpace(m.config.OhMyCode.Workspace)
	if workspace == "" {
		return "", "", errors.New("agents.ohMyCode.workspace is required")
	}

	script := strings.TrimSpace(m.config.OhMyCode.AgentManagerScript)
	if script == "" {
		script = defaultOhMyCodeAgentManagerScript
	}
	if !filepath.IsAbs(script) {
		script = filepath.Join(workspace, script)
	}

	return workspace, script, nil
}

func (m *Manager) validateOhMyCodeAgent(agentName string) (string, error) {
	name := strings.TrimSpace(agentName)
	if name == "" {
		return "", errors.New("agent name is required")
	}
	if err := channels.ValidateAgentName(name); err != nil {
		return "", err
	}
	allowlist := channels.NewAgentAllowlist(m.config.OhMyCode.AllowedAgents)
	if err := allowlist.Validate(name, m.config.OhMyCode.DefaultAgent); err != nil {
		return "", err
	}
	return name, nil
}

func runOhMyCodeAgentManager(ctx context.Context, workspace, script, stdin string, args ...string) (string, error) {
	cmdArgs := append([]string{script}, args...)
	cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
	cmd.Dir = workspace

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outText := strings.TrimSpace(stdout.String())
	errText := strings.TrimSpace(stderr.String())

	if err != nil {
		if errText != "" {
			return "", fmt.Errorf("oh-my-code agent-manager failed: %s", errText)
		}
		if outText != "" {
			return "", fmt.Errorf("oh-my-code agent-manager failed: %s", outText)
		}
		return "", fmt.Errorf("oh-my-code agent-manager failed: %w", err)
	}

	if outText == "" {
		outText = errText
	}
	return outText, nil
}

func buildOhMyCodeTaskPrompt(userText string) string {
	return fmt.Sprintf("Telegram user message:\n%s\n", strings.TrimSpace(userText))
}

func extractMonitorSnapshot(monitorOutput string) string {
	lines := strings.Split(monitorOutput, "\n")
	firstSep := -1
	secondSep := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "====") {
			if firstSep == -1 {
				firstSep = i
				continue
			}
			secondSep = i
			break
		}
	}

	if firstSep == -1 || secondSep == -1 || secondSep <= firstSep {
		return strings.TrimSpace(monitorOutput)
	}

	snapshot := strings.Join(lines[firstSep+1:secondSep], "\n")
	return strings.TrimSpace(snapshot)
}
