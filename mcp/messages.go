package mcp

import (
	"bytes"
	"encoding/json"

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

// ToolAnnotations provides hints about tool behavior per MCP 2026-07-28 spec.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// Tool represents a tool definition
type Tool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	InputSchema  InputSchema      `json:"inputSchema"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	OutputSchema map[string]any   `json:"outputSchema,omitempty"`
}

// InputSchema represents the JSON schema for tool input
type InputSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Required   []string       `json:"required"`
	Title      string         `json:"title,omitempty"`
	// Extras preserves the remaining JSON Schema 2020-12 vocabulary verbatim.
	Extras map[string]any `json:"-"`
}

// MarshalJSON preserves the full JSON Schema 2020-12 vocabulary while keeping
// the existing strongly-typed fields used by production tools.
func (s InputSchema) MarshalJSON() ([]byte, error) {
	result := make(map[string]any, len(s.Extras)+4)
	for key, value := range s.Extras {
		result[key] = value
	}
	result["type"] = s.Type
	properties := s.Properties
	if properties == nil {
		properties = map[string]any{}
	}
	required := s.Required
	if required == nil {
		required = []string{}
	}
	result["properties"] = properties
	result["required"] = required
	if s.Title != "" {
		result["title"] = s.Title
	}
	return json.Marshal(result)
}

// UnmarshalJSON restores strongly typed fields and retains every other JSON
// Schema 2020-12 keyword in Extras for lossless round trips.
func (s *InputSchema) UnmarshalJSON(data []byte) error {
	*s = InputSchema{}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if value, ok := raw["type"]; ok {
		if err := json.Unmarshal(value, &s.Type); err != nil {
			return err
		}
		delete(raw, "type")
	}
	if value, ok := raw["properties"]; ok {
		if err := json.Unmarshal(value, &s.Properties); err != nil {
			return err
		}
		delete(raw, "properties")
	}
	if value, ok := raw["required"]; ok {
		if err := json.Unmarshal(value, &s.Required); err != nil {
			return err
		}
		delete(raw, "required")
	}
	if value, ok := raw["title"]; ok {
		if err := json.Unmarshal(value, &s.Title); err != nil {
			return err
		}
		delete(raw, "title")
	}
	s.Extras = make(map[string]any, len(raw))
	for key, value := range raw {
		var decoded any
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			return err
		}
		s.Extras[key] = decoded
	}
	return nil
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
	if code > jsonrpc.ErrServerError || code < -32099 {
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
