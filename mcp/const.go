package mcp

// Protocol version
const (
	ProtocolVersion = "2026-07-28"
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
