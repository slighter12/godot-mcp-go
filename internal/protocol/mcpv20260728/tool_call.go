package mcpv20260728

import (
	"encoding/json"
	"errors"
	"strings"
)

var ErrInvalidToolCallParams = errors.New("invalid tools/call parameters")

// ToolCallParams is the single decoded representation used by transport
// header validation and tool execution.
type ToolCallParams struct {
	Name                  string
	Arguments             map[string]any
	Meta                  map[string]any
	InputResponses        map[string]any
	InputResponsesRaw     json.RawMessage
	InputResponsesPresent bool
	RequestState          string
}

// DecodeToolCallParams preserves the legacy tool alias only outside the
// released modern protocol.
func DecodeToolCallParams(raw json.RawMessage, modern bool) (ToolCallParams, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return ToolCallParams{}, ErrInvalidToolCallParams
	}
	var wire struct {
		Name         string         `json:"name"`
		Tool         string         `json:"tool"`
		Arguments    map[string]any `json:"arguments"`
		Meta         map[string]any `json:"_meta"`
		RequestState string         `json:"requestState"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ToolCallParams{}, ErrInvalidToolCallParams
	}
	if modern {
		if _, present := fields["tool"]; present || strings.TrimSpace(wire.Name) == "" {
			return ToolCallParams{}, ErrInvalidToolCallParams
		}
	} else if strings.TrimSpace(wire.Name) == "" {
		wire.Name = wire.Tool
	}

	result := ToolCallParams{
		Name: strings.TrimSpace(wire.Name), Arguments: wire.Arguments, Meta: wire.Meta,
		RequestState: wire.RequestState,
	}
	if rawResponses, present := fields["inputResponses"]; present {
		result.InputResponsesRaw = append(json.RawMessage(nil), rawResponses...)
		var err error
		result.InputResponses, result.InputResponsesPresent, err = DecodeInputResponses(rawResponses)
		if err != nil {
			return ToolCallParams{}, ErrInvalidToolCallParams
		}
	}
	return result, nil
}
