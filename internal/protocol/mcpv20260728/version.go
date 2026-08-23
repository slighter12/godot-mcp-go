package mcpv20260728

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/slighter12/godot-mcp-go/mcp"
)

const ProtocolVersion = mcp.ProtocolVersion

const (
	GodotExtensionID          = "com.slighter12/godot-mcp"
	CommandStreamExtensionID  = "com.slighter12/godot-mcp-command-stream"
	CommandNotificationMethod = "notifications/godot/command"
)

var (
	ErrMissingProtocolVersion    = errors.New("_meta.io.modelcontextprotocol/protocolVersion is required")
	ErrMissingClientCapabilities = errors.New("_meta.io.modelcontextprotocol/clientCapabilities is required")
	ErrInvalidProtocolVersion    = errors.New("unsupported MCP protocol version")
	ErrInvalidRequestMeta        = errors.New("invalid MCP request metadata")
	ErrInvalidHeaderEncoding     = errors.New("invalid MCP header encoding")
)

const (
	base64HeaderPrefix = "=?base64?"
	base64HeaderSuffix = "?="
)

// UnsupportedProtocolVersionError preserves the version supplied by a
// client while remaining compatible with errors.Is(err,
// ErrInvalidProtocolVersion).
type UnsupportedProtocolVersionError struct {
	Requested string
}

func (e *UnsupportedProtocolVersionError) Error() string {
	return fmt.Sprintf("unsupported MCP protocol version %q", e.Requested)
}

func (e *UnsupportedProtocolVersionError) Unwrap() error {
	return ErrInvalidProtocolVersion
}

// RequestMeta is the per-request protocol metadata required by MCP 2026-07-28.
// Raw retains namespaced extension metadata for application-specific handling.
type RequestMeta struct {
	ProtocolVersion    string
	ClientInfo         map[string]any
	ClientCapabilities map[string]any
	ProgressToken      any
	Raw                map[string]any
}

func IsSupportedProtocolVersion(version string) bool {
	return strings.TrimSpace(version) == ProtocolVersion
}

// DecodeHeaderValue decodes the MCP Base64 sentinel form used by standard
// HTTP header values such as Mcp-Name. Outer HTTP optional whitespace is
// ignored; whitespace inside the decoded value is preserved.
func DecodeHeaderValue(headerValue string) (string, error) {
	value := strings.TrimSpace(headerValue)
	hasPrefix := strings.HasPrefix(value, base64HeaderPrefix)
	hasSuffix := strings.HasSuffix(value, base64HeaderSuffix)
	if !hasPrefix {
		return value, nil
	}
	if !hasSuffix {
		return "", fmt.Errorf("%w: malformed Base64 sentinel", ErrInvalidHeaderEncoding)
	}

	encoded := strings.TrimSuffix(strings.TrimPrefix(value, base64HeaderPrefix), base64HeaderSuffix)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidHeaderEncoding, err)
	}
	return string(decoded), nil
}

func ParseRequestMeta(paramsRaw json.RawMessage) (RequestMeta, error) {
	trimmed := bytes.TrimSpace(paramsRaw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return RequestMeta{}, ErrInvalidRequestMeta
	}

	var params map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &params); err != nil {
		return RequestMeta{}, ErrInvalidRequestMeta
	}
	rawMeta, ok := params["_meta"]
	if !ok || len(bytes.TrimSpace(rawMeta)) == 0 {
		return RequestMeta{}, ErrInvalidRequestMeta
	}
	var meta map[string]any
	if err := json.Unmarshal(rawMeta, &meta); err != nil || meta == nil {
		return RequestMeta{}, ErrInvalidRequestMeta
	}

	version, ok := meta["io.modelcontextprotocol/protocolVersion"].(string)
	version = strings.TrimSpace(version)
	if !ok || version == "" {
		return RequestMeta{}, ErrMissingProtocolVersion
	}
	if !IsSupportedProtocolVersion(version) {
		return RequestMeta{ProtocolVersion: version, Raw: meta}, &UnsupportedProtocolVersionError{Requested: version}
	}

	capabilities, ok := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any)
	if !ok || capabilities == nil {
		return RequestMeta{ProtocolVersion: version, Raw: meta}, ErrMissingClientCapabilities
	}

	clientInfo, _ := meta["io.modelcontextprotocol/clientInfo"].(map[string]any)
	return RequestMeta{
		ProtocolVersion:    version,
		ClientInfo:         clientInfo,
		ClientCapabilities: capabilities,
		ProgressToken:      meta["progressToken"],
		Raw:                meta,
	}, nil
}

func ExtensionSettings(capabilities map[string]any, extensionID string) map[string]any {
	if capabilities == nil {
		return nil
	}
	extensions, _ := capabilities["extensions"].(map[string]any)
	if extensions == nil {
		return nil
	}
	settings, _ := extensions[extensionID].(map[string]any)
	return settings
}

func HasExtensionCapability(capabilities map[string]any, extensionID string) bool {
	return ExtensionSettings(capabilities, extensionID) != nil
}

func MutatingCapability(meta RequestMeta) bool {
	settings := ExtensionSettings(meta.ClientCapabilities, GodotExtensionID)
	if settings == nil {
		return false
	}
	allowed, _ := settings["mutating"].(bool)
	return allowed
}

func UnsupportedVersionData(requested string) map[string]any {
	return map[string]any{
		"supported": []string{ProtocolVersion},
		"requested": strings.TrimSpace(requested),
	}
}

func HeaderMismatchData(headerName, headerValue, bodyValue string) map[string]any {
	return map[string]any{
		"header":      headerName,
		"headerValue": strings.TrimSpace(headerValue),
		"bodyValue":   strings.TrimSpace(bodyValue),
	}
}

func ValidateMethodHeader(method, headerValue string) error {
	if strings.TrimSpace(headerValue) == "" || strings.TrimSpace(method) != strings.TrimSpace(headerValue) {
		return fmt.Errorf("Mcp-Method must match request method")
	}
	return nil
}

func ValidateNameHeader(method, name, headerValue string) error {
	requiresName := method == "tools/call" || method == "resources/read" || method == "prompts/get"
	if !requiresName {
		return nil
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(headerValue) == "" {
		return fmt.Errorf("Mcp-Name must match request name")
	}
	trimmedHeaderValue := strings.TrimSpace(headerValue)
	encoded := strings.HasPrefix(trimmedHeaderValue, base64HeaderPrefix) && strings.HasSuffix(trimmedHeaderValue, base64HeaderSuffix)
	if !encoded && !isSafeLiteralHeaderValue(trimmedHeaderValue) {
		return fmt.Errorf("Mcp-Name must use Base64 sentinel for unsafe values")
	}
	decoded, err := DecodeHeaderValue(headerValue)
	if err != nil || decoded != name {
		return fmt.Errorf("Mcp-Name must match request name")
	}
	return nil
}

func isSafeLiteralHeaderValue(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r > 0x7e || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// AddSupportedVersionsForInitialize adds the modern-only support hint to an
// error returned for a legacy initialize request. Legacy clients have no
// fall-forward mechanism, so this is their actionable version diagnostic.
func AddSupportedVersionsForInitialize(method string, data map[string]any) map[string]any {
	if method != "initialize" {
		return data
	}
	result := make(map[string]any, len(data)+1)
	for key, value := range data {
		result[key] = value
	}
	result["supported"] = []string{ProtocolVersion}
	return result
}
