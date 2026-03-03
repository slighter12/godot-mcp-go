package mcpv20251125

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

const ProtocolVersion = "2025-11-25"

var (
	ErrMissingProtocolVersion = errors.New("initialize.params.protocolVersion is required")
	ErrInvalidProtocolVersion = errors.New("initialize.params.protocolVersion must be 2025-11-25")
)

func IsSupportedProtocolVersion(version string) bool {
	return strings.TrimSpace(version) == ProtocolVersion
}

// IsSupportedProtocolHeader validates MCP-Protocol-Version header values.
// Some clients may send duplicated header values, which are merged as comma-separated tokens.
// This server only accepts 2025-11-25, so every token must match that version.
func IsSupportedProtocolHeader(headerValue string) bool {
	trimmedHeader := strings.TrimSpace(headerValue)
	if trimmedHeader == "" {
		return false
	}

	parts := strings.Split(trimmedHeader, ",")
	if len(parts) == 0 {
		return false
	}

	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return false
		}
		if !IsSupportedProtocolVersion(token) {
			return false
		}
	}
	return true
}

func ValidateInitializeProtocolVersion(paramsRaw json.RawMessage) error {
	trimmedParams := bytes.TrimSpace(paramsRaw)
	if len(trimmedParams) == 0 {
		return ErrMissingProtocolVersion
	}

	var params map[string]json.RawMessage
	if err := json.Unmarshal(trimmedParams, &params); err != nil {
		return ErrInvalidProtocolVersion
	}

	rawVersion, exists := params["protocolVersion"]
	if !exists {
		return ErrMissingProtocolVersion
	}

	rawVersion = bytes.TrimSpace(rawVersion)
	if len(rawVersion) == 0 {
		return ErrMissingProtocolVersion
	}

	var protocolVersion string
	if err := json.Unmarshal(rawVersion, &protocolVersion); err != nil {
		return ErrInvalidProtocolVersion
	}

	trimmed := strings.TrimSpace(protocolVersion)
	if trimmed == "" {
		return ErrMissingProtocolVersion
	}
	if !IsSupportedProtocolVersion(trimmed) {
		return ErrInvalidProtocolVersion
	}
	return nil
}
