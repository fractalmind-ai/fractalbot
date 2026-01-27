package gateway

import (
	"log"
	"sync"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
	"github.com/gorilla/websocket"
)

// Client represents a connected WebSocket client
type Client struct {
	ID        string
	Conn      *websocket.Conn
	Server    *Server
	sendLock  sync.Mutex
	closeChan chan struct{}
}

// NewClient creates a new client
func NewClient(id string, conn *websocket.Conn, server *Server) *Client {
	return &Client{
		ID:        id,
		Conn:      conn,
		Server:    server,
		closeChan: make(chan struct{}),
	}
}

// Handle processes incoming messages from client
func (c *Client) Handle() {
	defer c.Close()

	for {
		select {
		case <-c.closeChan:
			return

		default:
			var msg protocol.Message
			if err := c.Conn.ReadJSON(&msg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("WebSocket error [%s]: %v", c.ID, err)
				}
				return
			}

			// Process message
			c.ProcessMessage(&msg)
		}
	}
}

// ProcessMessage handles incoming message based on type
func (c *Client) ProcessMessage(msg *protocol.Message) {
	if msg == nil {
		return
	}

	switch msg.Kind {
	case protocol.MessageKindAgent:
		c.handleAgentMessage(msg)
	case protocol.MessageKindChannel:
		c.handleChannelMessage(msg)
	case protocol.MessageKindEvent:
		c.handleEventMessage(msg)
	case protocol.MessageKindTool:
		c.handleToolMessage(msg)
	default:
		log.Printf("Unknown message kind: %s", msg.Kind)
	}
}

// handleEventMessage processes event messages.
func (c *Client) handleEventMessage(msg *protocol.Message) {
	switch msg.Action {
	case protocol.ActionEcho:
		resp := protocol.Message{
			Kind:   protocol.MessageKindEvent,
			Action: protocol.ActionEcho,
			Data:   msg.Data,
		}
		if err := c.Send(&resp); err != nil {
			log.Printf("Echo send error [%s]: %v", c.ID, err)
		}
	default:
		log.Printf("Unknown event action: %s", msg.Action)
	}
}

// handleAgentMessage processes agent-related messages
func (c *Client) handleAgentMessage(msg *protocol.Message) {
	agentMgr := c.Server.GetAgentManager()

	switch msg.Action {
	case protocol.ActionCreate:
		// Create new agent session
	case protocol.ActionStart:
		// Start agent
	case protocol.ActionStop:
		// Stop agent
	case protocol.ActionList:
		// List agents
		resp := protocol.Message{
			Kind:   protocol.MessageKindAgent,
			Action: protocol.ActionList,
			Data:   agentMgr.List(),
		}
		c.Send(&resp)
	default:
		log.Printf("Unknown agent action: %s", msg.Action)
	}
}

// handleChannelMessage processes channel-related messages
func (c *Client) handleChannelMessage(msg *protocol.Message) {
	log.Printf("Channel message [%s]: %s", c.ID, msg.Action)
	// TODO: implement channel message handling
}

// handleToolMessage processes tool execution messages
func (c *Client) handleToolMessage(msg *protocol.Message) {
	log.Printf("Tool message [%s]: %s", c.ID, msg.Action)
	// TODO: implement tool execution
}

// Send sends a message to client
func (c *Client) Send(msg *protocol.Message) error {
	c.sendLock.Lock()
	defer c.sendLock.Unlock()

	return c.Conn.WriteJSON(msg)
}

// Close closes the client connection
func (c *Client) Close() {
	select {
	case <-c.closeChan:
		// Already closing
		return
	default:
		close(c.closeChan)
		c.Conn.Close()
		log.Printf("🔌 Client disconnected: %s", c.ID)
	}
}
