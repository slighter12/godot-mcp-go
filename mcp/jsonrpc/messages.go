package jsonrpc

import "encoding/json"

// Request represents a JSON-RPC request
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC response
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error represents a JSON-RPC error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Notification represents a JSON-RPC notification (request without id)
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// NewResponse creates a new JSON-RPC response
func NewResponse(id any, result any) *Response {
	return &Response{
		JSONRPC: Version,
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates a new JSON-RPC error response
func NewErrorResponse(id any, code int, message string, data any) *Response {
	return &Response{
		JSONRPC: Version,
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// NewNotification creates a new JSON-RPC notification
func NewNotification(method string, params any) *Notification {
	return &Notification{
		JSONRPC: Version,
		Method:  method,
		Params:  params,
	}
}
