// Package agentruntime defines the runtime-neutral delivery contract shared by
// the Agent Manager and autonomous schedulers.
package agentruntime

import (
	"context"
	"time"
)

const (
	OhMyCode      = "ohMyCode"
	CodexAppCDP   = "codexAppCDP"
	ClaudeDesktop = "claudeDesktop"
)

// DispatchRequest is an internal agent wakeup that is not associated with a
// messaging channel.
type DispatchRequest struct {
	Runtime      string
	Agent        string
	Text         string
	Source       string
	JobID        string
	RunID        string
	ScheduledAt  time.Time
	ExpiresAt    time.Time
	CoalesceKey  string
	CronProfiles []string
}

// DispatchResult reports whether a runtime accepted, queued, or rejected a
// wakeup. It does not imply that the agent completed the resulting work.
type DispatchResult struct {
	Runtime    string
	Agent      string
	Status     string
	EnvelopeID string
	InboxPath  string
	Error      string
}

// Dispatcher routes a request to an explicitly selected Agent Runtime.
type Dispatcher interface {
	DispatchRuntime(ctx context.Context, request DispatchRequest) DispatchResult
}
