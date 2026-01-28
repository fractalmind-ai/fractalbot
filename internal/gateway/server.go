package gateway

import (
	"context"
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
}

// NewServer creates a new gateway server
func NewServer(cfg *config.Config) (*Server, error) {
	if cfg.Gateway == nil {
		return nil, fmt.Errorf("gateway config is required")
	}

	// Initialize channels
	channelManager := channels.NewManager(cfg.Channels)

	// Initialize agent manager
	agentManager := agent.NewManager(cfg.Agents)
	agentManager.ChannelManager = channelManager

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

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	// Disconnect all clients
	s.clientsMutex.Lock()
	for _, client := range s.clients {
		client.Close()
	}
	s.clientsMutex.Unlock()

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
