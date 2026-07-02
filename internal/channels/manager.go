package channels

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

// Manager starts and stops configured channels.
type Manager struct {
	cfg       *config.ChannelsConfig
	agentsCfg *config.AgentsConfig
	handler   IncomingMessageHandler

	channels map[string]Channel
	workers  map[string]*channelWorker

	startMu      sync.Mutex
	startCancels map[string]context.CancelFunc
}

// NewManager creates a new channel manager.
func NewManager(cfg *config.ChannelsConfig, agentsCfg *config.AgentsConfig) *Manager {
	return &Manager{
		cfg:       cfg,
		agentsCfg: agentsCfg,
		channels:  make(map[string]Channel),
		workers:   make(map[string]*channelWorker),

		startCancels: make(map[string]context.CancelFunc),
	}
}

// SetHandler sets the inbound message handler used by channels.
func (m *Manager) SetHandler(handler IncomingMessageHandler) {
	m.handler = handler
	for _, channel := range m.channels {
		if handlerAware, ok := channel.(HandlerAware); ok {
			handlerAware.SetHandler(handler)
		}
	}
}

// Register adds a channel to the manager.
func (m *Manager) Register(channel Channel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	name := channel.Name()
	if name == "" {
		return errors.New("channel name is required")
	}
	if _, exists := m.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}
	if m.handler != nil {
		if handlerAware, ok := channel.(HandlerAware); ok {
			handlerAware.SetHandler(m.handler)
		}
	}
	m.channels[name] = channel
	return nil
}

// Get returns a channel by name.
func (m *Manager) Get(name string) Channel {
	return m.channels[name]
}

// List returns registered channels in name order.
func (m *Manager) List() []Channel {
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	channels := make([]Channel, 0, len(names))
	for _, name := range names {
		channels = append(channels, m.channels[name])
	}
	return channels
}

// Start starts configured channels.
// Each channel gets an independent context (not derived from the parent) so
// that a crash or context cancellation in one channel cannot cascade to others.
// The parent ctx is only used to trigger graceful shutdown of all channels.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.registerConfiguredChannels(); err != nil {
		return err
	}

	for _, channel := range m.List() {
		if channel.IsRunning() {
			continue
		}
		name := channel.Name()
		if m.hasInFlightStart(name) {
			continue
		}
		// Independent context per channel — isolates crash/cancel from other channels.
		channelCtx, cancel := context.WithCancel(context.Background())
		m.trackInFlightStart(name, cancel)

		// Create and start per-channel worker
		w := newChannelWorker(channel)
		m.workers[name] = w
		w.start(channelCtx)

		go m.startChannel(name, channel, channelCtx)
	}

	return nil
}

// Stop stops configured channels and their workers.
func (m *Manager) Stop() error {
	m.cancelInFlightStarts()

	// Stop workers first so no new sends are attempted
	for name, w := range m.workers {
		w.stop()
		delete(m.workers, name)
	}

	var errs []error
	for _, channel := range m.List() {
		if !channel.IsRunning() {
			continue
		}
		if err := channel.Stop(context.Background()); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", channel.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop channels: %v", errs)
	}

	return nil
}

// Send routes an outbound message through the channel's worker.
// It performs a synchronous send with rate limiting and returns result metadata.
// Returns error if the channel is not found.
func (m *Manager) Send(ctx context.Context, channelName string, msg OutboundMessage) (*SendResult, error) {
	w, ok := m.workers[channelName]
	if !ok {
		// Fallback to direct send if worker not started yet
		ch := m.Get(channelName)
		if ch == nil {
			return nil, fmt.Errorf("channel %q not found", channelName)
		}
		return ch.Send(ctx, msg)
	}
	return w.sendSync(ctx, msg)
}

func (m *Manager) startChannel(name string, channel Channel, ctx context.Context) {
	defer m.clearInFlightStart(name)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("channel %s panicked: %v", name, r)
		}
	}()
	if err := channel.Start(ctx); err != nil {
		log.Printf("channel %s failed to start: %v", name, err)
	}
}

func (m *Manager) hasInFlightStart(name string) bool {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	_, ok := m.startCancels[name]
	return ok
}

func (m *Manager) trackInFlightStart(name string, cancel context.CancelFunc) {
	m.startMu.Lock()
	m.startCancels[name] = cancel
	m.startMu.Unlock()
}

func (m *Manager) clearInFlightStart(name string) {
	m.startMu.Lock()
	delete(m.startCancels, name)
	m.startMu.Unlock()
}

func (m *Manager) cancelInFlightStarts() {
	m.startMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(m.startCancels))
	for name, cancel := range m.startCancels {
		cancels = append(cancels, cancel)
		delete(m.startCancels, name)
	}
	m.startMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (m *Manager) registerConfiguredChannels() error {
	if m.cfg == nil {
		return nil
	}

	if m.cfg.Telegram != nil && m.cfg.Telegram.Enabled {
		if m.Get("telegram") != nil {
			return nil
		}
		if m.cfg.Telegram.BotToken == "" {
			return errors.New("channels.telegram.botToken is required when telegram is enabled")
		}

		defaultAgent, allowedAgents, agentConfigName := activeChannelAgentConfig(m.agentsCfg)
		if err := validateOhMyCodeAgentConfig(defaultAgent, allowedAgents); err != nil {
			return fmt.Errorf("invalid %s config: %w", agentConfigName, err)
		}

		bot, err := NewTelegramBot(m.cfg.Telegram.BotToken, m.cfg.Telegram.AllowedUsers, m.cfg.Telegram.AdminID, defaultAgent, allowedAgents)
		if err != nil {
			return fmt.Errorf("failed to init telegram bot: %w", err)
		}
		bot.setAllowedChats(m.cfg.Telegram.AllowedChats)

		bot.ConfigureMode(m.cfg.Telegram.Mode)
		bot.ConfigurePolling(
			m.cfg.Telegram.PollingTimeoutSeconds,
			m.cfg.Telegram.PollingLimit,
			m.cfg.Telegram.PollingOffsetFile,
		)
		bot.ConfigureWebhook(
			m.cfg.Telegram.WebhookListenAddr,
			m.cfg.Telegram.WebhookPath,
			m.cfg.Telegram.WebhookPublicURL,
			m.cfg.Telegram.WebhookSecretToken,
		)
		bot.ConfigureWebhookLifecycle(
			m.cfg.Telegram.WebhookRegisterOnStart,
			m.cfg.Telegram.WebhookDeleteOnStop,
		)

		if err := m.Register(bot); err != nil {
			return fmt.Errorf("failed to register telegram bot: %w", err)
		}
	}

	if m.cfg.Feishu != nil && m.cfg.Feishu.Enabled {
		if m.Get("feishu") != nil {
			return nil
		}
		if strings.TrimSpace(m.cfg.Feishu.AppID) == "" || strings.TrimSpace(m.cfg.Feishu.AppSecret) == "" {
			return errors.New("channels.feishu.appId and channels.feishu.appSecret are required when feishu is enabled")
		}

		defaultAgent, allowedAgents, agentConfigName := activeChannelAgentConfig(m.agentsCfg)
		if err := validateOhMyCodeAgentConfig(defaultAgent, allowedAgents); err != nil {
			return fmt.Errorf("invalid %s config: %w", agentConfigName, err)
		}

		bot, err := NewFeishuBot(
			m.cfg.Feishu.AppID,
			m.cfg.Feishu.AppSecret,
			m.cfg.Feishu.Domain,
			m.cfg.Feishu.AllowedUsers,
			defaultAgent,
			allowedAgents,
		)
		if err != nil {
			return fmt.Errorf("failed to init feishu bot: %w", err)
		}

		if err := m.Register(bot); err != nil {
			return fmt.Errorf("failed to register feishu bot: %w", err)
		}
	}

	if m.cfg.Slack != nil && m.cfg.Slack.Enabled {
		if m.Get("slack") != nil {
			return nil
		}
		if strings.TrimSpace(m.cfg.Slack.BotToken) == "" || strings.TrimSpace(m.cfg.Slack.AppToken) == "" {
			return errors.New("channels.slack.botToken and channels.slack.appToken are required when slack is enabled")
		}

		defaultAgent, allowedAgents, agentConfigName := activeChannelAgentConfig(m.agentsCfg)
		if err := validateOhMyCodeAgentConfig(defaultAgent, allowedAgents); err != nil {
			return fmt.Errorf("invalid %s config: %w", agentConfigName, err)
		}

		bot, err := NewSlackBot(
			m.cfg.Slack.BotToken,
			m.cfg.Slack.AppToken,
			m.cfg.Slack.AllowedUsers,
			m.cfg.Slack.AllowedChannels,
			defaultAgent,
			allowedAgents,
		)
		if err != nil {
			return fmt.Errorf("failed to init slack bot: %w", err)
		}

		if err := m.Register(bot); err != nil {
			return fmt.Errorf("failed to register slack bot: %w", err)
		}
	}

	if m.cfg.Discord != nil && m.cfg.Discord.Enabled {
		if m.Get("discord") != nil {
			return nil
		}
		if strings.TrimSpace(m.cfg.Discord.Token) == "" {
			return errors.New("channels.discord.token is required when discord is enabled")
		}

		defaultAgent, allowedAgents, agentConfigName := activeChannelAgentConfig(m.agentsCfg)
		if err := validateOhMyCodeAgentConfig(defaultAgent, allowedAgents); err != nil {
			return fmt.Errorf("invalid %s config: %w", agentConfigName, err)
		}

		bot, err := NewDiscordBot(
			m.cfg.Discord.Token,
			m.cfg.Discord.AllowedUsers,
			defaultAgent,
			allowedAgents,
		)
		if err != nil {
			return fmt.Errorf("failed to init discord bot: %w", err)
		}

		if err := m.Register(bot); err != nil {
			return fmt.Errorf("failed to register discord bot: %w", err)
		}
	}

	if m.cfg.IMessage != nil && m.cfg.IMessage.Enabled {
		if m.Get("imessage") != nil {
			return nil
		}
		if strings.TrimSpace(m.cfg.IMessage.Recipient) == "" {
			return errors.New("channels.imessage.recipient is required when imessage is enabled")
		}

		bot, err := NewIMessageBot(
			m.cfg.IMessage.Recipient,
			m.cfg.IMessage.Message,
			m.cfg.IMessage.Service,
		)
		if err != nil {
			return fmt.Errorf("failed to init imessage bot: %w", err)
		}
		bot.ConfigurePolling(
			m.cfg.IMessage.PollingEnabled,
			m.cfg.IMessage.PollingIntervalSeconds,
			m.cfg.IMessage.PollingLimit,
			m.cfg.IMessage.DatabasePath,
		)

		if err := m.Register(bot); err != nil {
			return fmt.Errorf("failed to register imessage bot: %w", err)
		}
	}

	if m.cfg.Demail != nil && m.cfg.Demail.Enabled {
		if m.Get("demail") != nil {
			return nil
		}
		if strings.TrimSpace(m.cfg.Demail.RPCURL) == "" || strings.TrimSpace(m.cfg.Demail.PackageID) == "" || strings.TrimSpace(m.cfg.Demail.Address) == "" {
			return errors.New("channels.demail.rpcUrl, channels.demail.packageId and channels.demail.address are required when demail is enabled")
		}

		channel, err := NewDemailChannel(DemailOptions{
			RPCURL:          m.cfg.Demail.RPCURL,
			PackageID:       m.cfg.Demail.PackageID,
			Address:         m.cfg.Demail.Address,
			IdentityKeyFile: m.cfg.Demail.IdentityKeyFile,
			SponsorAddress:  m.cfg.Demail.SponsorAddress,
			GasCoin:         m.cfg.Demail.GasCoin,
			PollInterval:    time.Duration(m.cfg.Demail.PollIntervalSeconds) * time.Second,
			AllowedSenders:  m.cfg.Demail.AllowedSenders,
			Peers:           m.cfg.Demail.Peers,
		})
		if err != nil {
			return fmt.Errorf("failed to init demail channel: %w", err)
		}

		if err := m.Register(channel); err != nil {
			return fmt.Errorf("failed to register demail channel: %w", err)
		}
	}

	return nil
}

func activeChannelAgentConfig(cfg *config.AgentsConfig) (string, []string, string) {
	if cfg == nil {
		return "", nil, "agents.ohMyCode"
	}
	router := strings.TrimSpace(cfg.Router)
	if router == "codexAppCDP" && cfg.CodexAppCDP != nil {
		return cfg.CodexAppCDP.DefaultAgent, cfg.CodexAppCDP.AllowedAgents, "agents.codexAppCDP"
	}
	if router == "" && cfg.CodexAppCDP != nil && cfg.CodexAppCDP.Enabled && (cfg.OhMyCode == nil || !cfg.OhMyCode.Enabled) {
		return cfg.CodexAppCDP.DefaultAgent, cfg.CodexAppCDP.AllowedAgents, "agents.codexAppCDP"
	}
	if cfg.OhMyCode != nil {
		return cfg.OhMyCode.DefaultAgent, cfg.OhMyCode.AllowedAgents, "agents.ohMyCode"
	}
	return "", nil, "agents.ohMyCode"
}

func validateOhMyCodeAgentConfig(defaultAgent string, allowedAgents []string) error {
	trimmedDefault := strings.TrimSpace(defaultAgent)
	if trimmedDefault != "" {
		if err := ValidateAgentName(trimmedDefault); err != nil {
			return err
		}
	}

	for _, agent := range allowedAgents {
		trimmed := strings.TrimSpace(agent)
		if trimmed == "" {
			continue
		}
		if err := ValidateAgentName(trimmed); err != nil {
			return err
		}
	}

	allowlist := NewAgentAllowlist(allowedAgents)
	if allowlist.configured {
		if trimmedDefault == "" {
			return fmt.Errorf("default agent is required when allowedAgents is configured")
		}
		if err := allowlist.Validate(trimmedDefault, trimmedDefault); err != nil {
			return err
		}
	}

	return nil
}
