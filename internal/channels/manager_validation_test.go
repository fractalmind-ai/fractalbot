package channels

import "testing"

func TestValidateOhMyCodeAgentConfig(t *testing.T) {
	cases := []struct {
		name         string
		defaultAgent string
		allowed      []string
		wantErr      bool
	}{
		{
			name:         "default-empty-allowed-empty",
			defaultAgent: "",
			allowed:      nil,
		},
		{
			name:         "default-empty-allowed-set",
			defaultAgent: "",
			allowed:      []string{"qa-1"},
			wantErr:      true,
		},
		{
			name:         "default-invalid",
			defaultAgent: "-bad",
			allowed:      nil,
			wantErr:      true,
		},
		{
			name:         "allowlist-invalid",
			defaultAgent: "qa-1",
			allowed:      []string{"-bad"},
			wantErr:      true,
		},
		{
			name:         "default-not-in-allowlist",
			defaultAgent: "qa-1",
			allowed:      []string{"coder-a"},
			wantErr:      true,
		},
		{
			name:         "default-in-allowlist",
			defaultAgent: "qa-1",
			allowed:      []string{"qa-1", "coder-a"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOhMyCodeAgentConfig(tc.defaultAgent, tc.allowed)
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
