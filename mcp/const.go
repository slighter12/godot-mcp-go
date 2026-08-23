package mcp

// Protocol version
const (
	ProtocolVersion = "2026-07-28"
	ServerVersion   = "0.3.0"
)

type MessageType string

// Message protocol types
const (
	TypeInit     MessageType = "init"
	TypeToolCall MessageType = "tool_call"
	TypeResult   MessageType = "result"
	TypeError    MessageType = "error"
	TypePing     MessageType = "ping"
	TypePong     MessageType = "pong"
)
