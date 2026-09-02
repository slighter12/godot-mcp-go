package mcpv20260728

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strings"

	"github.com/slighter12/godot-mcp-go/mcp"
)

const (
	MaxDecodedContentBlockBytes = 8 << 20
	MaxSerializedContentBytes   = 16 << 20
	MaxSerializedCompleteBytes  = 16 << 20
	maxContentBlocks            = 128
)

// ValidateCompleteResult rejects invalid or unbounded standard MCP content
// before a handler result crosses a transport boundary.
func ValidateCompleteResult(result mcp.CompleteResult) error {
	if len(result.Content) > maxContentBlocks {
		return errors.New("too many content blocks")
	}
	serialized, err := json.Marshal(result.Content)
	if err != nil {
		return errors.New("content is not JSON encodable")
	}
	if len(serialized) > MaxSerializedContentBytes {
		return errors.New("serialized content exceeds limit")
	}
	for index, rawBlock := range result.Content {
		if err := validateContentBlock(rawBlock); err != nil {
			return fmt.Errorf("content block %d: %w", index, err)
		}
	}
	return nil
}

// NormalizeCompleteResult converts any JSON object result into the released
// complete-result envelope without mutating the handler-owned value.
func NormalizeCompleteResult(value any, defaultMeta map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("complete result is not JSON encodable")
	}
	if len(raw) > MaxSerializedCompleteBytes {
		return nil, errors.New("complete result exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil || result == nil {
		return nil, errors.New("complete result must be an object")
	}
	if rawType, present := result["resultType"]; present {
		resultType, ok := rawType.(string)
		if !ok || strings.TrimSpace(resultType) != "" && resultType != "complete" {
			return nil, errors.New("complete result has invalid resultType")
		}
	}
	result["resultType"] = "complete"
	if rawMeta, present := result["_meta"]; !present || rawMeta == nil {
		result["_meta"] = defaultMeta
	} else if _, ok := rawMeta.(map[string]any); !ok {
		return nil, errors.New("complete result metadata must be an object")
	}
	if content, present := result["content"]; present && content == nil {
		result["content"] = []any{}
	}
	if _, present := result["content"]; present {
		encodedContentResult, err := json.Marshal(result)
		if err != nil {
			return nil, errors.New("complete result content is not JSON encodable")
		}
		var contentResult mcp.CompleteResult
		if err := json.Unmarshal(encodedContentResult, &contentResult); err != nil {
			return nil, errors.New("complete result content is invalid")
		}
		if err := ValidateCompleteResult(contentResult); err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > MaxSerializedCompleteBytes {
		return nil, errors.New("complete result exceeds limit")
	}
	return result, nil
}

// NormalizeMethodCompleteResult applies the released complete-result contract
// for the JSON-RPC method that produced value.
func NormalizeMethodCompleteResult(method string, value any, defaultMeta map[string]any) (map[string]any, error) {
	result, err := NormalizeCompleteResult(value, defaultMeta)
	if err != nil {
		return nil, err
	}
	switch method {
	case "tools/call":
		if _, present := result["content"]; !present {
			result["content"] = []any{}
		}
	case "prompts/get":
		if err := validatePromptMessages(result["messages"]); err != nil {
			return nil, err
		}
		if description, present := result["description"]; present {
			if _, ok := description.(string); !ok {
				return nil, errors.New("prompt description must be a string")
			}
		}
	case "resources/read":
		if err := validateResourceResultContents(result["contents"]); err != nil {
			return nil, err
		}
		if ttl, present := result["ttlMs"]; present {
			if !validNonNegativeInteger(ttl) {
				return nil, errors.New("resource ttlMs must be a non-negative integer")
			}
		} else {
			result["ttlMs"] = json.Number("0")
		}
		if scope, present := result["cacheScope"]; present {
			if scope != "private" && scope != "public" {
				return nil, errors.New("resource cacheScope is invalid")
			}
		} else {
			result["cacheScope"] = "private"
		}
	default:
		return nil, errors.New("unsupported complete-result method")
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > MaxSerializedCompleteBytes {
		return nil, errors.New("complete result exceeds limit")
	}
	return result, nil
}

func validatePromptMessages(value any) error {
	messages, ok := value.([]any)
	if !ok {
		return errors.New("prompt messages are required")
	}
	if len(messages) > maxContentBlocks {
		return errors.New("too many prompt messages")
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			return errors.New("prompt message must be an object")
		}
		role, _ := message["role"].(string)
		if role != "user" && role != "assistant" {
			return errors.New("prompt message role is invalid")
		}
		if err := validateContentBlock(message["content"]); err != nil {
			return fmt.Errorf("prompt message content: %w", err)
		}
	}
	return nil
}

func validateResourceResultContents(value any) error {
	contents, ok := value.([]any)
	if !ok {
		return errors.New("resource contents are required")
	}
	if len(contents) > maxContentBlocks {
		return errors.New("too many resource contents")
	}
	for _, raw := range contents {
		content, ok := raw.(map[string]any)
		if !ok {
			return errors.New("resource content must be an object")
		}
		if err := validateResourceContents(content); err != nil {
			return err
		}
	}
	return nil
}

func validNonNegativeInteger(value any) bool {
	switch number := value.(type) {
	case json.Number:
		integer, err := number.Int64()
		return err == nil && integer >= 0
	case float64:
		return number >= 0 && number == float64(int64(number))
	case int64:
		return number >= 0
	case int:
		return number >= 0
	default:
		return false
	}
}

func validateContentBlock(rawBlock any) error {
	raw, err := json.Marshal(rawBlock)
	if err != nil {
		return errors.New("block is not JSON encodable")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var block map[string]any
	if decoder.Decode(&block) != nil || block == nil {
		return errors.New("block must be an object")
	}
	typeName, ok := requiredString(block, "type")
	if !ok {
		return errors.New("block type is required")
	}
	switch typeName {
	case "text":
		text, ok := stringField(block, "text")
		if !ok || len(text) > MaxDecodedContentBlockBytes {
			return errors.New("text content is invalid or too large")
		}
	case "image", "audio":
		data, dataOK := stringField(block, "data")
		mediaType, mimeOK := requiredString(block, "mimeType")
		if !dataOK || !mimeOK || !validMediaType(mediaType, typeName) || validateStrictBase64(data) != nil {
			return errors.New("binary content is invalid")
		}
	case "resource":
		resource, ok := block["resource"].(map[string]any)
		if !ok {
			return errors.New("embedded resource is required")
		}
		if err := validateResourceContents(resource); err != nil {
			return err
		}
	case "resource_link":
		uri, uriOK := requiredString(block, "uri")
		_, nameOK := requiredString(block, "name")
		if !uriOK || !nameOK || !validAbsoluteURI(uri) {
			return errors.New("resource link is invalid")
		}
		if value, present := block["mimeType"]; present {
			mediaType, ok := value.(string)
			if !ok || !validMediaType(mediaType, "") {
				return errors.New("resource link MIME type is invalid")
			}
		}
	default:
		return errors.New("unsupported content block type")
	}
	return nil
}

func validateResourceContents(resource map[string]any) error {
	uri, ok := requiredString(resource, "uri")
	if !ok || !validAbsoluteURI(uri) {
		return errors.New("embedded resource URI is invalid")
	}
	text, hasText := resource["text"]
	blob, hasBlob := resource["blob"]
	if hasText == hasBlob {
		return errors.New("embedded resource must contain exactly one of text or blob")
	}
	if hasText {
		value, ok := text.(string)
		if !ok || len(value) > MaxDecodedContentBlockBytes {
			return errors.New("embedded text is invalid or too large")
		}
	} else {
		value, ok := blob.(string)
		if !ok || validateStrictBase64(value) != nil {
			return errors.New("embedded blob is invalid")
		}
	}
	if value, present := resource["mimeType"]; present {
		mediaType, ok := value.(string)
		if !ok || !validMediaType(mediaType, "") {
			return errors.New("embedded resource MIME type is invalid")
		}
	}
	return nil
}

func validateStrictBase64(value string) error {
	if len(value) > ((MaxDecodedContentBlockBytes+2)/3)*4 {
		return errors.New("base64 content exceeds limit")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) > MaxDecodedContentBlockBytes || base64.StdEncoding.EncodeToString(decoded) != value {
		return errors.New("invalid base64 content")
	}
	return nil
}

func validMediaType(value, requiredPrefix string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || mediaType == "" {
		return false
	}
	return requiredPrefix == "" || strings.HasPrefix(strings.ToLower(mediaType), requiredPrefix+"/")
}

func validAbsoluteURI(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && parsed.Scheme != ""
}

func requiredString(object map[string]any, key string) (string, bool) {
	value, ok := stringField(object, key)
	return value, ok && value != ""
}

func stringField(object map[string]any, key string) (string, bool) {
	value, ok := object[key].(string)
	return value, ok
}
