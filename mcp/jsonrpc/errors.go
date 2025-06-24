package jsonrpc

import "errors"

type ErrorCode int

// JSON-RPC 2.0 Error Codes
const (
	// Standard JSON-RPC 2.0 error codes
	ErrParseError     ErrorCode = -32700 // Invalid JSON was received by the server
	ErrInvalidRequest ErrorCode = -32600 // The JSON sent is not a valid Request object
	ErrMethodNotFound ErrorCode = -32601 // The method does not exist / is not available
	ErrInvalidParams  ErrorCode = -32602 // Invalid method parameter(s)
	ErrInternalError  ErrorCode = -32603 // Internal JSON-RPC error

	// Server error codes (-32000 to -32099)
	ErrServerError ErrorCode = -32000 // Reserved for implementation-defined server-errors
)

// JSONRPCError represents a JSON-RPC error
type JSONRPCError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Data    any       `json:"data,omitempty"`
}

// NewJSONRPCError creates a new JSON-RPC error
func NewJSONRPCError(code ErrorCode, message string, data any) *JSONRPCError {
	return &JSONRPCError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// Error implements the error interface
func (e *JSONRPCError) Error() string {
	return e.Message
}

// IsParseError checks if the error is a parse error
func IsParseError(err error) bool {
	return IsError(err, ErrParseError)
}

// IsInvalidRequest checks if the error is an invalid request error
func IsInvalidRequest(err error) bool {
	return IsError(err, ErrInvalidRequest)
}

// IsMethodNotFound checks if the error is a method not found error
func IsMethodNotFound(err error) bool {
	return IsError(err, ErrMethodNotFound)
}

// IsInvalidParams checks if the error is an invalid params error
func IsInvalidParams(err error) bool {
	return IsError(err, ErrInvalidParams)
}

// IsInternalError checks if the error is an internal error
func IsInternalError(err error) bool {
	return IsError(err, ErrInternalError)
}

// IsServerError checks if the error is a server error
func IsServerError(err error) bool {
	return IsError(err, ErrServerError)
}

// IsError checks if the error is a JSON-RPC error with the given code
func IsError(err error, code ErrorCode) bool {
	var e *JSONRPCError
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}
