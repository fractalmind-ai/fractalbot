package channels

import "testing"

func TestParseMonitorArgs(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		wantAgent string
		wantLines int
		wantErr   bool
	}{
		{
			name:      "default-lines",
			text:      "/monitor qa-1",
			wantAgent: "qa-1",
			wantLines: defaultMonitorLines,
		},
		{
			name:      "explicit-lines",
			text:      "/monitor qa-1 5",
			wantAgent: "qa-1",
			wantLines: 5,
		},
		{
			name:      "cap-lines",
			text:      "/monitor qa-1 500",
			wantAgent: "qa-1",
			wantLines: maxMonitorLines,
		},
		{
			name:    "missing-agent",
			text:    "/monitor",
			wantErr: true,
		},
		{
			name:    "invalid-lines",
			text:    "/monitor qa-1 nope",
			wantErr: true,
		},
		{
			name:    "zero-lines",
			text:    "/monitor qa-1 0",
			wantErr: true,
		},
		{
			name:    "too-many-args",
			text:    "/monitor qa-1 5 extra",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent, lines, err := parseMonitorArgs(splitCommand(tc.text))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if agent != tc.wantAgent {
				t.Fatalf("agent=%q want %q", agent, tc.wantAgent)
			}
			if lines != tc.wantLines {
				t.Fatalf("lines=%d want %d", lines, tc.wantLines)
			}
		})
	}
}

func TestValidateAgentCommandName(t *testing.T) {
	cases := []struct {
		name         string
		agent        string
		defaultAgent string
		allowlist    []string
		wantErr      bool
	}{
		{
			name:         "allowed-configured",
			agent:        "coder-a",
			defaultAgent: "qa-1",
			allowlist:    []string{"qa-1", "coder-a"},
		},
		{
			name:         "blocked-configured",
			agent:        "coder-b",
			defaultAgent: "qa-1",
			allowlist:    []string{"qa-1", "coder-a"},
			wantErr:      true,
		},
		{
			name:         "default-only",
			agent:        "qa-1",
			defaultAgent: "qa-1",
		},
		{
			name:         "default-only-block",
			agent:        "coder-a",
			defaultAgent: "qa-1",
			wantErr:      true,
		},
		{
			name:         "invalid-name",
			agent:        "-bad",
			defaultAgent: "qa-1",
			allowlist:    []string{"qa-1", "-bad"},
			wantErr:      true,
		},
		{
			name:         "missing-default",
			agent:        "qa-1",
			defaultAgent: "",
			wantErr:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowlist := NewAgentAllowlist(tc.allowlist)
			err := validateAgentCommandName(tc.agent, tc.defaultAgent, allowlist)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
