package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/agentruntime"
)

// DispatchRuntime delivers a non-channel wakeup to an explicitly selected
// Agent Runtime. The result confirms delivery or queueing, not task completion.
func (m *Manager) DispatchRuntime(ctx context.Context, request agentruntime.DispatchRequest) agentruntime.DispatchResult {
	request.Runtime = strings.TrimSpace(request.Runtime)
	request.Agent = strings.TrimSpace(request.Agent)
	request.Text = strings.TrimSpace(request.Text)
	result := agentruntime.DispatchResult{Runtime: request.Runtime, Agent: request.Agent}
	if request.Text == "" {
		result.Status = "error"
		result.Error = "runtime dispatch text is required"
		return result
	}

	switch request.Runtime {
	case agentruntime.OhMyCode:
		return m.dispatchOhMyCodeRuntime(ctx, request)
	case agentruntime.CodexAppCDP:
		return m.dispatchCodexAppRuntime(ctx, request)
	case agentruntime.ClaudeDesktop:
		return m.dispatchClaudeDesktopRuntime(ctx, request)
	default:
		result.Status = "error"
		result.Error = fmt.Sprintf("unsupported Agent Runtime %q", request.Runtime)
		return result
	}
}

func (m *Manager) dispatchOhMyCodeRuntime(ctx context.Context, request agentruntime.DispatchRequest) agentruntime.DispatchResult {
	result := agentruntime.DispatchResult{Runtime: request.Runtime, Agent: request.Agent}
	workspace, script, err := m.resolveOhMyCodeWorkspaceAndScript()
	if err != nil {
		return runtimeDispatchError(result, err)
	}
	name, err := m.validateOhMyCodeAgent(request.Agent)
	if err != nil {
		return runtimeDispatchError(result, err)
	}
	result.Agent = name

	timeout := defaultOhMyCodeAssignTimeout
	if m.config.OhMyCode.AssignTimeoutSeconds > 0 {
		timeout = time.Duration(m.config.OhMyCode.AssignTimeoutSeconds) * time.Second
	}
	dispatchCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		dispatchCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if _, err := runOhMyCodeAgentManager(dispatchCtx, workspace, script, buildRuntimePrompt(request), "assign", name); err != nil {
		return runtimeDispatchError(result, err)
	}
	result.Status = "assigned"
	return result
}

func (m *Manager) dispatchCodexAppRuntime(ctx context.Context, request agentruntime.DispatchRequest) agentruntime.DispatchResult {
	result := agentruntime.DispatchResult{Runtime: request.Runtime, Agent: request.Agent}
	if m.config == nil || m.config.CodexAppCDP == nil || !m.config.CodexAppCDP.Enabled {
		return runtimeDispatchError(result, errors.New("agents.codexAppCDP is not enabled"))
	}
	name, err := m.validateCodexAppAgent(request.Agent)
	if err != nil {
		return runtimeDispatchError(result, err)
	}
	result.Agent = name
	envelope := buildRuntimeAppEnvelope(request, name)
	delivery := m.deliverCodexAppEnvelope(ctx, m.config.CodexAppCDP, envelope, buildRuntimePrompt(request))
	result.Status = delivery.Status
	result.EnvelopeID = delivery.EnvelopeID
	result.InboxPath = delivery.InboxPath
	if delivery.Error != nil {
		result.Error = delivery.Error.Error()
	}
	return result
}

func (m *Manager) dispatchClaudeDesktopRuntime(ctx context.Context, request agentruntime.DispatchRequest) agentruntime.DispatchResult {
	result := agentruntime.DispatchResult{Runtime: request.Runtime, Agent: request.Agent}
	if m.config == nil || m.config.ClaudeDesktop == nil || !m.config.ClaudeDesktop.Enabled {
		return runtimeDispatchError(result, errors.New("agents.claudeDesktop is not enabled"))
	}
	name, err := m.validateClaudeDesktopAgent(request.Agent)
	if err != nil {
		return runtimeDispatchError(result, err)
	}
	result.Agent = name
	envelope := buildRuntimeAppEnvelope(request, name)
	delivery := m.deliverClaudeDesktopEnvelope(ctx, m.config.ClaudeDesktop, envelope, buildRuntimePrompt(request))
	result.Status = delivery.Status
	result.EnvelopeID = delivery.EnvelopeID
	result.InboxPath = delivery.InboxPath
	if delivery.Error != nil {
		result.Error = delivery.Error.Error()
	}
	return result
}

func runtimeDispatchError(result agentruntime.DispatchResult, err error) agentruntime.DispatchResult {
	result.Status = "error"
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func buildRuntimeAppEnvelope(request agentruntime.DispatchRequest, agentName string) InboundAppEnvelope {
	receivedAt := time.Now().UTC()
	scheduledAt := request.ScheduledAt
	if scheduledAt.IsZero() {
		scheduledAt = receivedAt
	}
	return InboundAppEnvelope{
		ID:            newEnvelopeID(),
		ReceivedAt:    receivedAt.Format(time.RFC3339Nano),
		Source:        strings.TrimSpace(request.Source),
		JobID:         strings.TrimSpace(request.JobID),
		RunID:         strings.TrimSpace(request.RunID),
		ScheduledAt:   scheduledAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:     formatOptionalRuntimeTime(request.ExpiresAt),
		CoalesceKey:   strings.TrimSpace(request.CoalesceKey),
		SelectedAgent: strings.TrimSpace(agentName),
		Text:          strings.TrimSpace(request.Text),
	}
}

func buildRuntimePrompt(request agentruntime.DispatchRequest) string {
	profiles := append([]string(nil), request.CronProfiles...)
	sort.Strings(profiles)
	var builder strings.Builder
	builder.WriteString("# FractalBot Heartbeat\n\n")
	builder.WriteString(fmt.Sprintf("- job_id: %s\n", strings.TrimSpace(request.JobID)))
	builder.WriteString(fmt.Sprintf("- run_id: %s\n", strings.TrimSpace(request.RunID)))
	builder.WriteString(fmt.Sprintf("- runtime: %s\n", strings.TrimSpace(request.Runtime)))
	builder.WriteString(fmt.Sprintf("- agent: %s\n", strings.TrimSpace(request.Agent)))
	if !request.ScheduledAt.IsZero() {
		builder.WriteString(fmt.Sprintf("- scheduled_at: %s\n", request.ScheduledAt.UTC().Format(time.RFC3339Nano)))
	}
	if !request.ExpiresAt.IsZero() {
		builder.WriteString(fmt.Sprintf("- expires_at: %s\n", request.ExpiresAt.UTC().Format(time.RFC3339Nano)))
	}
	builder.WriteString("\nThis is an autonomous wakeup, not a chat message. Do not reply to a messaging channel unless the task itself requires it.\n\n")
	builder.WriteString("Instruction:\n")
	builder.WriteString(strings.TrimSpace(request.Text))
	builder.WriteString("\n")
	if len(profiles) > 0 {
		builder.WriteString("\nIf and only if there is no actionable work, you may reduce this heartbeat frequency with:\n")
		builder.WriteString(fmt.Sprintf("fractalbot heartbeat cron set --job %s --profile <profile> --reason \"no actionable tasks\"\n", strings.TrimSpace(request.JobID)))
		builder.WriteString("Allowed profiles: ")
		builder.WriteString(strings.Join(profiles, ", "))
		builder.WriteString("\n")
	}
	return builder.String()
}

func formatOptionalRuntimeTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

var _ agentruntime.Dispatcher = (*Manager)(nil)
