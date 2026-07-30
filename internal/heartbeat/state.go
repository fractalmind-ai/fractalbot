package heartbeat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const persistedStateVersion = 1

type persistedState struct {
	Version int                          `json:"version"`
	Jobs    map[string]persistedJobState `json:"jobs"`
}

type persistedJobState struct {
	EffectiveProfile   string    `json:"effective_profile,omitempty"`
	NextRunAt          time.Time `json:"next_run_at,omitempty"`
	InFlight           bool      `json:"in_flight,omitempty"`
	LastScheduledAt    time.Time `json:"last_scheduled_at,omitempty"`
	LastDispatchAt     time.Time `json:"last_dispatch_at,omitempty"`
	LastDispatchStatus string    `json:"last_dispatch_status,omitempty"`
	LastDispatchError  string    `json:"last_dispatch_error,omitempty"`
	LastEnvelopeID     string    `json:"last_envelope_id,omitempty"`
	LastInboxPath      string    `json:"last_inbox_path,omitempty"`
	ScheduleReason     string    `json:"schedule_reason,omitempty"`
	ScheduleUpdatedBy  string    `json:"schedule_updated_by,omitempty"`
	ScheduleUpdatedAt  time.Time `json:"schedule_updated_at,omitempty"`
}

func decodePersistedState(data []byte) (persistedState, error) {
	var state persistedState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return persistedState{}, err
	}
	if state.Version != persistedStateVersion {
		return persistedState{}, fmt.Errorf("unsupported heartbeat state version %d", state.Version)
	}
	if state.Jobs == nil {
		state.Jobs = make(map[string]persistedJobState)
	}
	return state, nil
}

func (s *Scheduler) saveStateLocked() error {
	if s == nil || s.config == nil || !s.config.Enabled {
		return nil
	}
	state := persistedState{Version: persistedStateVersion, Jobs: make(map[string]persistedJobState, len(s.jobs))}
	for id, job := range s.jobs {
		state.Jobs[id] = job.state
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode heartbeat state: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(s.statePath)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("create heartbeat state directory: %w", err)
	}
	tmp, err := os.CreateTemp(directory, ".heartbeat-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create heartbeat state temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set heartbeat state permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write heartbeat state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync heartbeat state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close heartbeat state: %w", err)
	}
	if err := os.Rename(tmpPath, s.statePath); err != nil {
		return fmt.Errorf("commit heartbeat state: %w", err)
	}
	return nil
}
