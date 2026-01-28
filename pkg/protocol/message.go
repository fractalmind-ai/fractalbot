package protocol

// MessageKind defines type of message
type MessageKind string

const (
	MessageKindAgent   MessageKind = "agent"
	MessageKindChannel MessageKind = "channel"
	MessageKindTool    MessageKind = "tool"
	MessageKindEvent   MessageKind = "event"
)

// Action defines action within a message kind
type Action string

const (
	ActionCreate  Action = "create"
	ActionStart   Action = "start"
	ActionStop    Action = "stop"
	ActionList    Action = "list"
	ActionAssign  Action = "assign"
	ActionMonitor Action = "monitor"
	ActionEcho    Action = "echo"
)

// Message represents a protocol message
type Message struct {
	Kind   MessageKind `json:"kind"`
	Action Action      `json:"action,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// AgentInfo contains information about an agent
type AgentInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Model  string `json:"model,omitempty"`
}

// ChannelInfo contains information about a channel
type ChannelInfo struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// ToolInfo contains information about a tool
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Response represents a response from gateway
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}
