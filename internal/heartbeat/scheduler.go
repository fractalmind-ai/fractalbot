// Package heartbeat schedules runtime-neutral agent wakeups.
package heartbeat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/agentruntime"
	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/robfig/cron/v3"
)

const (
	defaultStateFilename = "heartbeat-state.json"
	defaultMaxConcurrent = 1
	defaultTickInterval  = time.Second
	maxDispatchAttempts  = 3
	initialRetryDelay    = time.Second
)

// Status is the redacted scheduler state exposed by the gateway.
type Status struct {
	Enabled       bool        `json:"enabled"`
	MaxConcurrent int         `json:"max_concurrent,omitempty"`
	Jobs          []JobStatus `json:"jobs,omitempty"`
}

// JobStatus contains schedule and delivery telemetry without instruction text.
type JobStatus struct {
	ID                    string `json:"id"`
	Runtime               string `json:"runtime"`
	Agent                 string `json:"agent"`
	ConfiguredCron        string `json:"configured_cron"`
	EffectiveProfile      string `json:"effective_profile,omitempty"`
	EffectiveCron         string `json:"effective_cron"`
	Timezone              string `json:"timezone"`
	NextRunAt             string `json:"next_run_at,omitempty"`
	InFlight              bool   `json:"in_flight"`
	LastScheduledAt       string `json:"last_scheduled_at,omitempty"`
	LastDispatchAt        string `json:"last_dispatch_at,omitempty"`
	LastDispatchStatus    string `json:"last_dispatch_status,omitempty"`
	LastDispatchError     string `json:"last_dispatch_error,omitempty"`
	LastEnvelopeID        string `json:"last_envelope_id,omitempty"`
	LastInboxPath         string `json:"last_inbox_path,omitempty"`
	LastScheduleReason    string `json:"last_schedule_reason,omitempty"`
	LastScheduleUpdatedBy string `json:"last_schedule_updated_by,omitempty"`
	LastScheduleUpdatedAt string `json:"last_schedule_updated_at,omitempty"`
}

type compiledJob struct {
	config           config.HeartbeatJobConfig
	defaultSchedule  cron.Schedule
	profileSchedules map[string]cron.Schedule
	state            persistedJobState
	inFlight         bool
}

// Scheduler owns cron state, dispatch concurrency, and profile overrides.
type Scheduler struct {
	config       *config.HeartbeatConfig
	dispatcher   agentruntime.Dispatcher
	statePath    string
	now          func() time.Time
	tickInterval time.Duration
	retryDelay   func(attempt int) time.Duration

	mu      sync.RWMutex
	jobs    map[string]*compiledJob
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
	ctx     context.Context

	semaphore  chan struct{}
	dispatchWg sync.WaitGroup
}

// New creates a scheduler and restores persisted profile overrides. Missed
// executions are intentionally ignored; every next run is computed from now.
func New(cfg *config.HeartbeatConfig, workspace string, dispatcher agentruntime.Dispatcher) (*Scheduler, error) {
	return newScheduler(cfg, workspace, dispatcher, time.Now, defaultTickInterval)
}

func newScheduler(cfg *config.HeartbeatConfig, workspace string, dispatcher agentruntime.Dispatcher, now func() time.Time, tickInterval time.Duration) (*Scheduler, error) {
	if cfg == nil {
		return nil, nil
	}
	if now == nil {
		now = time.Now
	}
	if tickInterval <= 0 {
		tickInterval = defaultTickInterval
	}
	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}
	scheduler := &Scheduler{
		config:       cfg,
		dispatcher:   dispatcher,
		statePath:    resolveStatePath(cfg.StatePath, workspace),
		now:          now,
		tickInterval: tickInterval,
		retryDelay:   dispatchRetryDelay,
		jobs:         make(map[string]*compiledJob, len(cfg.Jobs)),
		semaphore:    make(chan struct{}, maxConcurrent),
	}
	if !cfg.Enabled {
		return scheduler, nil
	}
	if dispatcher == nil {
		return nil, errors.New("heartbeat dispatcher is required")
	}

	for idx := range cfg.Jobs {
		jobConfig := cfg.Jobs[idx]
		defaultSchedule, err := parseSchedule(jobConfig.Cron, jobConfig.Timezone)
		if err != nil {
			return nil, fmt.Errorf("compile heartbeat job %q: %w", jobConfig.ID, err)
		}
		job := &compiledJob{
			config:           jobConfig,
			defaultSchedule:  defaultSchedule,
			profileSchedules: make(map[string]cron.Schedule, len(jobConfig.AgentCronProfiles)),
		}
		for profile, expression := range jobConfig.AgentCronProfiles {
			schedule, err := parseSchedule(expression, jobConfig.Timezone)
			if err != nil {
				return nil, fmt.Errorf("compile heartbeat job %q profile %q: %w", jobConfig.ID, profile, err)
			}
			job.profileSchedules[profile] = schedule
		}
		scheduler.jobs[jobConfig.ID] = job
	}

	if err := scheduler.loadState(); err != nil {
		return nil, err
	}
	base := scheduler.now()
	for _, job := range scheduler.jobs {
		job.state.NextRunAt = job.effectiveSchedule().Next(base)
		job.state.InFlight = false
	}
	return scheduler, nil
}

// Start launches the cron loop. It is safe to call Start once.
func (s *Scheduler) Start(ctx context.Context) error {
	if s == nil || s.config == nil || !s.config.Enabled {
		return nil
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	s.started = true
	runCtx := s.ctx
	done := s.done
	s.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(s.tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				s.runDue(s.now())
			}
		}
	}()
	return nil
}

// Stop cancels scheduling and waits for in-flight dispatch calls.
func (s *Scheduler) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	done := s.done
	s.started = false
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	waitDone := make(chan struct{})
	go func() {
		s.dispatchWg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetProfile applies an operator-approved profile and recalculates the next run.
func (s *Scheduler) SetProfile(jobID, profile, reason, updatedBy string) (JobStatus, error) {
	if s == nil || s.config == nil || !s.config.Enabled {
		return JobStatus{}, errors.New("heartbeat scheduler is disabled")
	}
	jobID = strings.TrimSpace(jobID)
	profile = strings.TrimSpace(profile)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return JobStatus{}, errors.New("schedule change reason is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return JobStatus{}, fmt.Errorf("heartbeat job %q not found", jobID)
	}
	if _, ok := job.profileSchedules[profile]; !ok {
		return JobStatus{}, fmt.Errorf("heartbeat profile %q is not allowed for job %q", profile, jobID)
	}
	if job.state.EffectiveProfile == profile {
		return s.jobStatusLocked(job), nil
	}
	previous := job.state
	now := s.now()
	job.state.EffectiveProfile = profile
	job.state.ScheduleReason = reason
	job.state.ScheduleUpdatedBy = strings.TrimSpace(updatedBy)
	job.state.ScheduleUpdatedAt = now
	job.state.NextRunAt = job.effectiveSchedule().Next(now)
	if err := s.saveStateLocked(); err != nil {
		job.state = previous
		return JobStatus{}, err
	}
	return s.jobStatusLocked(job), nil
}

// ResetProfile restores a job to its configured cron.
func (s *Scheduler) ResetProfile(jobID, reason, updatedBy string) (JobStatus, error) {
	if s == nil || s.config == nil || !s.config.Enabled {
		return JobStatus{}, errors.New("heartbeat scheduler is disabled")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok {
		return JobStatus{}, fmt.Errorf("heartbeat job %q not found", strings.TrimSpace(jobID))
	}
	if job.state.EffectiveProfile == "" {
		return s.jobStatusLocked(job), nil
	}
	previous := job.state
	now := s.now()
	job.state.EffectiveProfile = ""
	job.state.ScheduleReason = strings.TrimSpace(reason)
	job.state.ScheduleUpdatedBy = strings.TrimSpace(updatedBy)
	job.state.ScheduleUpdatedAt = now
	job.state.NextRunAt = job.defaultSchedule.Next(now)
	if err := s.saveStateLocked(); err != nil {
		job.state = previous
		return JobStatus{}, err
	}
	return s.jobStatusLocked(job), nil
}

// ResetForInbound restores matching idle jobs after a normal inbound message
// was successfully routed to the same Runtime and Agent.
func (s *Scheduler) ResetForInbound(runtimeName, agentName string) {
	if s == nil || s.config == nil || !s.config.Enabled {
		return
	}
	runtimeName = strings.TrimSpace(runtimeName)
	agentName = strings.TrimSpace(agentName)
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	now := s.now()
	previous := make(map[string]persistedJobState)
	for id, job := range s.jobs {
		if !job.config.ResetCronOnInbound || job.config.Runtime != runtimeName || job.config.Agent != agentName || job.state.EffectiveProfile == "" {
			continue
		}
		previous[id] = job.state
		job.state.EffectiveProfile = ""
		job.state.ScheduleReason = "normal inbound activity"
		job.state.ScheduleUpdatedBy = "gateway"
		job.state.ScheduleUpdatedAt = now
		job.state.NextRunAt = job.defaultSchedule.Next(now)
		changed = true
	}
	if changed {
		if err := s.saveStateLocked(); err != nil {
			for id, state := range previous {
				s.jobs[id].state = state
			}
			log.Printf("heartbeat: persist inbound cron reset: %v", err)
		}
	}
}

// Status returns a stable, redacted snapshot.
func (s *Scheduler) Status() *Status {
	if s == nil || s.config == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := &Status{Enabled: s.config.Enabled, MaxConcurrent: cap(s.semaphore)}
	ids := make([]string, 0, len(s.jobs))
	for id := range s.jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		status.Jobs = append(status.Jobs, s.jobStatusLocked(s.jobs[id]))
	}
	return status
}

func (s *Scheduler) runDue(now time.Time) {
	if s == nil || s.config == nil || !s.config.Enabled {
		return
	}
	type dueDispatch struct {
		jobID   string
		request agentruntime.DispatchRequest
	}
	var due []dueDispatch

	s.mu.Lock()
	for id, job := range s.jobs {
		if job.state.NextRunAt.IsZero() {
			job.state.NextRunAt = job.effectiveSchedule().Next(now)
			continue
		}
		if job.state.NextRunAt.After(now) {
			continue
		}
		scheduledAt := job.state.NextRunAt
		job.state.NextRunAt = job.effectiveSchedule().Next(now)
		job.state.LastScheduledAt = scheduledAt
		if job.inFlight {
			job.state.LastDispatchStatus = "skipped_overlap"
			continue
		}
		select {
		case s.semaphore <- struct{}{}:
			job.inFlight = true
			job.state.InFlight = true
			runID := newRunID()
			profiles := make([]string, 0, len(job.profileSchedules))
			for profile := range job.profileSchedules {
				profiles = append(profiles, profile)
			}
			due = append(due, dueDispatch{
				jobID: id,
				request: agentruntime.DispatchRequest{
					Runtime:      job.config.Runtime,
					Agent:        job.config.Agent,
					Text:         job.config.Text,
					Source:       "heartbeat",
					JobID:        id,
					RunID:        runID,
					ScheduledAt:  scheduledAt,
					ExpiresAt:    job.state.NextRunAt,
					CoalesceKey:  "heartbeat:" + id,
					CronProfiles: profiles,
				},
			})
		default:
			job.state.LastDispatchStatus = "skipped_capacity"
		}
	}
	if err := s.saveStateLocked(); err != nil {
		log.Printf("heartbeat: persist scheduled state: %v", err)
	}
	dispatchContext := s.ctx
	if dispatchContext == nil {
		dispatchContext = context.Background()
	}
	for _, item := range due {
		s.dispatchWg.Add(1)
		go s.dispatch(dispatchContext, item.jobID, item.request)
	}
	s.mu.Unlock()
}

func (s *Scheduler) dispatch(ctx context.Context, jobID string, request agentruntime.DispatchRequest) {
	defer s.dispatchWg.Done()
	defer func() { <-s.semaphore }()
	result := agentruntime.DispatchResult{Status: "error", Error: "heartbeat dispatch did not run"}
	for attempt := 1; attempt <= maxDispatchAttempts; attempt++ {
		result = s.dispatcher.DispatchRuntime(ctx, request)
		if strings.TrimSpace(result.Status) != "error" || attempt == maxDispatchAttempts {
			break
		}
		delay := s.retryDelay(attempt)
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			result.Status = "error"
			result.Error = ctx.Err().Error()
			attempt = maxDispatchAttempts
		case <-timer.C:
		}
	}
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	job.inFlight = false
	job.state.InFlight = false
	job.state.LastDispatchAt = now
	job.state.LastDispatchStatus = strings.TrimSpace(result.Status)
	job.state.LastDispatchError = strings.TrimSpace(result.Error)
	job.state.LastEnvelopeID = strings.TrimSpace(result.EnvelopeID)
	job.state.LastInboxPath = strings.TrimSpace(result.InboxPath)
	if err := s.saveStateLocked(); err != nil {
		log.Printf("heartbeat: persist dispatch result: %v", err)
	}
}

func dispatchRetryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	return initialRetryDelay << (attempt - 1)
}

func (job *compiledJob) effectiveSchedule() cron.Schedule {
	if schedule, ok := job.profileSchedules[job.state.EffectiveProfile]; ok {
		return schedule
	}
	return job.defaultSchedule
}

func (job *compiledJob) effectiveCron() string {
	if expression, ok := job.config.AgentCronProfiles[job.state.EffectiveProfile]; ok {
		return expression
	}
	return job.config.Cron
}

func (s *Scheduler) jobStatusLocked(job *compiledJob) JobStatus {
	return JobStatus{
		ID:                    job.config.ID,
		Runtime:               job.config.Runtime,
		Agent:                 job.config.Agent,
		ConfiguredCron:        job.config.Cron,
		EffectiveProfile:      job.state.EffectiveProfile,
		EffectiveCron:         job.effectiveCron(),
		Timezone:              job.config.Timezone,
		NextRunAt:             formatTime(job.state.NextRunAt),
		InFlight:              job.state.InFlight,
		LastScheduledAt:       formatTime(job.state.LastScheduledAt),
		LastDispatchAt:        formatTime(job.state.LastDispatchAt),
		LastDispatchStatus:    job.state.LastDispatchStatus,
		LastDispatchError:     job.state.LastDispatchError,
		LastEnvelopeID:        job.state.LastEnvelopeID,
		LastInboxPath:         job.state.LastInboxPath,
		LastScheduleReason:    job.state.ScheduleReason,
		LastScheduleUpdatedBy: job.state.ScheduleUpdatedBy,
		LastScheduleUpdatedAt: formatTime(job.state.ScheduleUpdatedAt),
	}
}

func parseSchedule(expression, timezone string) (cron.Schedule, error) {
	return cron.ParseStandard("CRON_TZ=" + strings.TrimSpace(timezone) + " " + strings.TrimSpace(expression))
}

func resolveStatePath(configuredPath, workspace string) string {
	if path := strings.TrimSpace(configuredPath); path != "" {
		return filepath.Clean(path)
	}
	if strings.TrimSpace(workspace) == "" {
		workspace = "./workspace"
	}
	return filepath.Join(filepath.Clean(workspace), defaultStateFilename)
}

func newRunID() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err == nil {
		return hex.EncodeToString(data)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *Scheduler) loadState() error {
	data, err := os.ReadFile(s.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read heartbeat state: %w", err)
	}
	state, err := decodePersistedState(data)
	if err != nil {
		return fmt.Errorf("decode heartbeat state: %w", err)
	}
	for id, persisted := range state.Jobs {
		job, ok := s.jobs[id]
		if !ok {
			continue
		}
		if persisted.EffectiveProfile != "" {
			if _, allowed := job.profileSchedules[persisted.EffectiveProfile]; !allowed {
				persisted.EffectiveProfile = ""
			}
		}
		persisted.InFlight = false
		job.state = persisted
	}
	return nil
}
