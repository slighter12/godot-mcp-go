package mcp

import "encoding/json"

// TextContent is a standard MCP text content block.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ImageContent is a standard base64-encoded MCP image content block.
type ImageContent struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// AudioContent is a standard base64-encoded MCP audio content block.
type AudioContent struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// ResourceContents is the text-or-blob payload embedded in a resource block.
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// EmbeddedResourceContent is a standard embedded resource content block.
type EmbeddedResourceContent struct {
	Type     string           `json:"type"`
	Resource ResourceContents `json:"resource"`
}

// ResourceLinkContent is a standard MCP resource link content block.
type ResourceLinkContent struct {
	Type        string `json:"type"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

// CompleteResult is the common shape for a completed 2026-07-28 operation.
type CompleteResult struct {
	ResultType        string         `json:"resultType"`
	Meta              map[string]any `json:"_meta,omitempty"`
	Content           []any          `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

// InputRequest describes one client operation needed before a request can finish.
type InputRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

// InputRequiredResult is the ephemeral MRTR response shape.
type InputRequiredResult struct {
	ResultType    string                  `json:"resultType"`
	Meta          map[string]any          `json:"_meta,omitempty"`
	InputRequests map[string]InputRequest `json:"inputRequests,omitempty"`
	RequestState  string                  `json:"requestState,omitempty"`
}

// RoundTripRequest is the normalized, untrusted input supplied when an
// explicitly MRTR-capable handler is entered or re-entered.
type RoundTripRequest struct {
	Method                string
	Name                  string
	URI                   string
	Arguments             map[string]any
	InputResponses        map[string]any
	InputResponsesPresent bool
	Round                 int
	Continuation          json.RawMessage
	ClientInfo            map[string]any
	ClientCapabilities    map[string]any
	PrincipalID           string
}

// InputRequiredSpec describes another client-input round. Continuation is
// server-private data sealed into requestState and is never emitted directly.
type InputRequiredSpec struct {
	InputRequests map[string]InputRequest
	Continuation  json.RawMessage
	// Deprecated: every input-required round now mints protected requestState.
	ProtectState bool
}

// RoundTripOutcome contains exactly one completed method result or one
// input-required specification. Complete retains the method's existing result
// type and is validated by the method-specific shared dispatcher.
type RoundTripOutcome struct {
	Complete      any
	InputRequired *InputRequiredSpec
}
