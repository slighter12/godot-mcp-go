package mcp

import (
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
)

// InitMessage represents the initialization message
type InitMessage struct {
	Type     string         `json:"type"`
	Version  string         `json:"version"`
	ClientID string         `json:"client_id,omitempty"`
	ServerID string         `json:"server_id,omitempty"`
	Tools    []Tool         `json:"tools"`
	Data     map[string]any `json:"data,omitempty"`
}

// Tool represents a tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema represents the JSON schema for tool input
type InputSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Required   []string       `json:"required"`
	Title      string         `json:"title"`
}

// ToolCallMessage represents a tool call request
type ToolCallMessage struct {
	Type      string         `json:"type"`
	ClientID  string         `json:"client_id,omitempty"`
	ServerID  string         `json:"server_id,omitempty"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// ResultMessage represents a successful tool execution result
type ResultMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id,omitempty"`
	ServerID string `json:"server_id,omitempty"`
	Tool     string `json:"tool"`
	Result   any    `json:"result"`
}

// ErrorMessage represents an error response
type ErrorMessage struct {
	Type     MessageType       `json:"type"`
	ClientID string            `json:"client_id,omitempty"`
	ServerID string            `json:"server_id,omitempty"`
	Message  string            `json:"message"`
	Code     jsonrpc.ErrorCode `json:"code"`
	Data     any               `json:"data,omitempty"`
}

// NewErrorMessage creates a new error message
func NewErrorMessage(clientID, serverID string, code jsonrpc.ErrorCode, message string, data any) *ErrorMessage {
	return &ErrorMessage{
		Type:     TypeError,
		ClientID: clientID,
		ServerID: serverID,
		Code:     code,
		Message:  message,
		Data:     data,
	}
}

// NewParseErrorMessage creates a new parse error message
func NewParseErrorMessage(clientID, serverID string, data any) *ErrorMessage {
	return NewErrorMessage(clientID, serverID, jsonrpc.ErrParseError, "Parse error", data)
}

// NewInvalidRequestMessage creates a new invalid request error message
func NewInvalidRequestMessage(clientID, serverID string, data any) *ErrorMessage {
	return NewErrorMessage(clientID, serverID, jsonrpc.ErrInvalidRequest, "Invalid request", data)
}

// NewMethodNotFoundMessage creates a new method not found error message
func NewMethodNotFoundMessage(clientID, serverID string, data any) *ErrorMessage {
	return NewErrorMessage(clientID, serverID, jsonrpc.ErrMethodNotFound, "Method not found", data)
}

// NewInvalidParamsMessage creates a new invalid params error message
func NewInvalidParamsMessage(clientID, serverID string, data any) *ErrorMessage {
	return NewErrorMessage(clientID, serverID, jsonrpc.ErrInvalidParams, "Invalid params", data)
}

// NewInternalErrorMessage creates a new internal error message
func NewInternalErrorMessage(clientID, serverID string, data any) *ErrorMessage {
	return NewErrorMessage(clientID, serverID, jsonrpc.ErrInternalError, "Internal error", data)
}

// NewServerErrorMessage creates a new server error message
func NewServerErrorMessage(clientID, serverID string, code jsonrpc.ErrorCode, message string, data any) *ErrorMessage {
	if code < jsonrpc.ErrServerError || code > -32099 {
		code = jsonrpc.ErrServerError
	}
	return NewErrorMessage(clientID, serverID, code, message, data)
}

// PingMessage represents a ping message
type PingMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id,omitempty"`
	ServerID string `json:"server_id,omitempty"`
}

// PongMessage represents a pong message
type PongMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id,omitempty"`
	ServerID string `json:"server_id,omitempty"`
}

// ToolProgressMessage represents a tool progress update
type ToolProgressMessage struct {
	Type     string  `json:"type"`
	ClientID string  `json:"client_id,omitempty"`
	ServerID string  `json:"server_id,omitempty"`
	Tool     string  `json:"tool"`
	Progress float64 `json:"progress"`
	Message  string  `json:"message,omitempty"`
}
