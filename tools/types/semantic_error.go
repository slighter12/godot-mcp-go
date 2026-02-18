package types

import (
	"errors"
	"fmt"
)

const (
	SemanticKindNotAvailable = "not_available"
)

// SemanticError marks tool failures that should be surfaced as structured isError payloads.
type SemanticError struct {
	Kind    string
	Message string
	Data    map[string]any
}

func (e *SemanticError) Error() string {
	if e == nil {
		return "tool semantic error"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Kind != "" {
		return fmt.Sprintf("tool semantic error: %s", e.Kind)
	}
	return "tool semantic error"
}

func NewSemanticError(kind, message string, data map[string]any) *SemanticError {
	return &SemanticError{Kind: kind, Message: message, Data: data}
}

func NewNotAvailableError(message string, data map[string]any) *SemanticError {
	if message == "" {
		message = "Tool is temporarily unavailable"
	}
	return NewSemanticError(SemanticKindNotAvailable, message, data)
}

func AsSemanticError(err error) (*SemanticError, bool) {
	if err == nil {
		return nil, false
	}
	var semanticErr *SemanticError
	if errors.As(err, &semanticErr) {
		return semanticErr, true
	}
	return nil, false
}
