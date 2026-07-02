package channels

import (
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestManagerRegistersDemailChannel(t *testing.T) {
	manager := NewManager(&config.ChannelsConfig{
		Demail: &config.DemailConfig{
			Enabled:   true,
			RPCURL:    "http://127.0.0.1:9000",
			PackageID: demailTestPackageID,
			Address:   demailTestAddress,
		},
	}, nil)

	if err := manager.registerConfiguredChannels(); err != nil {
		t.Fatalf("registerConfiguredChannels: %v", err)
	}

	channel := manager.Get("demail")
	if channel == nil {
		t.Fatal("expected demail channel to be registered")
	}
	if channel.Name() != "demail" {
		t.Fatalf("expected channel name demail, got %q", channel.Name())
	}
}

func TestManagerRejectsDemailWithoutRequiredFields(t *testing.T) {
	manager := NewManager(&config.ChannelsConfig{
		Demail: &config.DemailConfig{
			Enabled: true,
			RPCURL:  "http://127.0.0.1:9000",
		},
	}, nil)

	err := manager.registerConfiguredChannels()
	if err == nil {
		t.Fatal("expected error for missing demail config fields")
	}
	if !strings.Contains(err.Error(), "channels.demail") {
		t.Fatalf("expected channels.demail error, got %v", err)
	}
}
