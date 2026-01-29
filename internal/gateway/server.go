package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/agent"
	"github.com/fractalmind-ai/fractalbot/internal/channels"
	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/gorilla/websocket"
)

// Server represents the gateway WebSocket server
type Server struct {
	config       *config.Config
	upgrader     websocket.Upgrader
	clients      map[string]*Client
	clientsMutex sync.RWMutex
	httpServer   *http.Server
	agentManager *agent.Manager
	startTime    time.Time
}

// NewServer creates a new gateway server
func NewServer(cfg *config.Config) (*Server, error) {
	if cfg.Gateway == nil {
		return nil, fmt.Errorf("gateway config is required")
	}

	// Initialize channels
	channelManager := channels.NewManager(cfg.Channels, cfg.Agents)

	// Initialize agent manager
	agentManager := agent.NewManager(cfg.Agents)
	agentManager.ChannelManager = channelManager
	channelManager.SetHandler(agentManager)

	return &Server{
		config: cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now (TODO: add origin check)
			},
		},
		clients:      make(map[string]*Client),
		agentManager: agentManager,
	}, nil
}

// Start starts the gateway server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Status endpoint
	mux.HandleFunc("/status", s.handleStatus)

	if s.startTime.IsZero() {
		s.startTime = time.Now()
	}

	// Start HTTP server
	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", s.config.Gateway.Bind, s.config.Gateway.Port),
		Handler:           mux,
		ErrorLog:          log.New(os.Stderr, "HTTP: ", log.LstdFlags),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("🌐 HTTP server listening on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start channels
	if s.agentManager.ChannelManager != nil {
		if err := s.agentManager.ChannelManager.Start(ctx); err != nil {
			return fmt.Errorf("failed to start channels: %w", err)
		}
	}

	// Start agent manager
	if err := s.agentManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent manager: %w", err)
	}

	<-ctx.Done()
	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.agentManager != nil {
		if s.agentManager.ChannelManager != nil {
			if err := s.agentManager.ChannelManager.Stop(); err != nil {
				return fmt.Errorf("failed to stop channels: %w", err)
			}
		}
		if err := s.agentManager.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop agent manager: %w", err)
		}
	}

	// Disconnect all clients
	clients := s.snapshotClients()
	for _, client := range clients {
		client.Close()
	}

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
	}

	return nil
}

// handleWebSocket handles incoming WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	conn.SetReadLimit(readLimit)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	clientID := r.URL.Query().Get("session")
	if clientID == "" {
		clientID = generateClientID()
	}

	client := NewClient(clientID, conn, s)

	s.clientsMutex.Lock()
	s.clients[clientID] = client
	s.clientsMutex.Unlock()

	log.Printf("🔌 Client connected: %s", clientID)

	// Handle client messages
	go client.Handle()
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// GetAgentManager returns the agent manager
func (s *Server) GetAgentManager() *agent.Manager {
	return s.agentManager
}

type statusResponse struct {
	Status        string `json:"status"`
	ActiveClients int    `json:"active_clients"`
	Uptime        string `json:"uptime"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Duration(0)
	if !s.startTime.IsZero() {
		uptime = time.Since(s.startTime)
	}

	resp := statusResponse{
		Status:        "ok",
		ActiveClients: s.activeClients(),
		Uptime:        uptime.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) activeClients() int {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	return len(s.clients)
}

func (s *Server) snapshotClients() []*Client {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	clients := make([]*Client, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	return clients
}

func (s *Server) removeClient(id string) {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()
	delete(s.clients, id)
}
