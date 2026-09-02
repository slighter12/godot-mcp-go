package mcpv20260728

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/slighter12/godot-mcp-go/mcp"
)

const (
	maxHeaderSchemaDepth = 64
	maxHeaderSchemaNodes = 10_000
)

type parameterHeaderBinding struct {
	Path   []string
	Suffix string
	Type   string
}

// ParameterHeaderError identifies a header-bound argument that failed closed.
type ParameterHeaderError struct {
	Path   string
	Reason string
}

func (e *ParameterHeaderError) Error() string { return e.Reason }

// ValidateParameterHeaderSchema verifies every x-mcp-header annotation using
// the static properties-only reachability rules from the HTTP transport spec.
func ValidateParameterHeaderSchema(schema mcp.InputSchema) error {
	_, err := parameterHeaderBindings(schema)
	return err
}

// ValidateParameterHeaders compares every statically reachable annotated
// argument with its corresponding Mcp-Param header.
func ValidateParameterHeaders(schema mcp.InputSchema, arguments map[string]any, headers http.Header) error {
	bindings, err := parameterHeaderBindings(schema)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		path := strings.Join(binding.Path, ".")
		bodyValue, present := nestedArgument(arguments, binding.Path)
		headerValues := headers.Values("Mcp-Param-" + binding.Suffix)
		if !present || bodyValue == nil {
			if len(headerValues) != 0 {
				return &ParameterHeaderError{Path: path, Reason: "header present for omitted argument"}
			}
			continue
		}
		if len(headerValues) != 1 {
			return &ParameterHeaderError{Path: path, Reason: "required parameter header is missing or duplicated"}
		}
		decoded, err := decodeParameterHeaderValue(headerValues[0])
		if err != nil {
			return &ParameterHeaderError{Path: path, Reason: "malformed parameter header"}
		}
		if !parameterHeaderValueMatches(binding.Type, decoded, bodyValue) {
			return &ParameterHeaderError{Path: path, Reason: "parameter header does not match request body"}
		}
	}
	return nil
}

func nestedArgument(arguments map[string]any, path []string) (any, bool) {
	var current any = arguments
	for _, segment := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func decodeParameterHeaderValue(value string) (string, error) {
	const maxMCPParameterHeaderBytes = 8 * 1024
	if len(value) > maxMCPParameterHeaderBytes {
		return "", errors.New("parameter header is too large")
	}
	if !strings.HasPrefix(value, base64HeaderPrefix) || !strings.HasSuffix(value, base64HeaderSuffix) {
		if strings.TrimSpace(value) != value || !isSafeParameterHeaderLiteral(value) {
			return "", errors.New("unsafe literal parameter header")
		}
		return value, nil
	}
	decoded, encoded, err := decodeBase64Sentinel(value, false)
	if err != nil {
		return "", err
	}
	if !encoded {
		return "", errors.New("invalid parameter header encoding")
	}
	if !utf8.ValidString(decoded) {
		return "", errors.New("parameter header is not valid UTF-8")
	}
	return decoded, nil
}

func isSafeParameterHeaderLiteral(value string) bool {
	for _, char := range value {
		if char < 0x20 || char > 0x7e {
			return false
		}
	}
	return true
}

func parameterHeaderValueMatches(schemaType, headerValue string, bodyValue any) bool {
	switch schemaType {
	case "string":
		value, ok := bodyValue.(string)
		return ok && headerValue == value
	case "integer":
		headerInteger, err := strconv.ParseInt(headerValue, 10, 64)
		if err != nil || headerInteger < -(1<<53)+1 || headerInteger > (1<<53)-1 {
			return false
		}
		bodyNumber, ok := bodyValue.(float64)
		return ok && math.Trunc(bodyNumber) == bodyNumber && bodyNumber == float64(headerInteger)
	case "boolean":
		bodyBoolean, ok := bodyValue.(bool)
		if !ok || (headerValue != "true" && headerValue != "false") {
			return false
		}
		return (headerValue == "true") == bodyBoolean
	default:
		return false
	}
}

func parameterHeaderBindings(schema mcp.InputSchema) ([]parameterHeaderBinding, error) {
	bindings := []parameterHeaderBinding{}
	seenSuffixes := map[string]string{}
	nodes := 0

	var containsAnnotation func(any, int) (bool, error)
	containsAnnotation = func(value any, depth int) (bool, error) {
		nodes++
		if depth > maxHeaderSchemaDepth || nodes > maxHeaderSchemaNodes {
			return false, errors.New("input schema exceeds x-mcp-header traversal limits")
		}
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == "x-mcp-header" {
					return true, nil
				}
				found, err := containsAnnotation(child, depth+1)
				if err != nil || found {
					return found, err
				}
			}
		case []any:
			for _, child := range typed {
				found, err := containsAnnotation(child, depth+1)
				if err != nil || found {
					return found, err
				}
			}
		}
		return false, nil
	}

	var visitProperty func(map[string]any, []string, int) error
	visitProperty = func(property map[string]any, path []string, depth int) error {
		nodes++
		if depth > maxHeaderSchemaDepth || nodes > maxHeaderSchemaNodes {
			return errors.New("input schema exceeds x-mcp-header traversal limits")
		}
		if rawSuffix, annotated := property["x-mcp-header"]; annotated {
			suffix, ok := rawSuffix.(string)
			if !ok || !IsHTTPToken(suffix) {
				return fmt.Errorf("property %q has an invalid header suffix", strings.Join(path, "."))
			}
			typeName, _ := property["type"].(string)
			if typeName != "string" && typeName != "integer" && typeName != "boolean" {
				return fmt.Errorf("property %q must have type string, integer, or boolean", strings.Join(path, "."))
			}
			canonical := strings.ToLower(suffix)
			if previous, duplicate := seenSuffixes[canonical]; duplicate {
				return fmt.Errorf("properties %q and %q use duplicate header suffixes", previous, strings.Join(path, "."))
			}
			seenSuffixes[canonical] = strings.Join(path, ".")
			// Copy before sibling traversal can reuse path's backing array.
			bindings = append(bindings, parameterHeaderBinding{Path: append([]string(nil), path...), Suffix: suffix, Type: typeName})
		}

		for key, child := range property {
			if key == "x-mcp-header" {
				continue
			}
			if key == "properties" {
				children, ok := child.(map[string]any)
				if !ok {
					continue
				}
				for name, rawChild := range children {
					childSchema, ok := rawChild.(map[string]any)
					if !ok {
						found, err := containsAnnotation(rawChild, depth+1)
						if err != nil {
							return err
						}
						if found {
							return fmt.Errorf("property %q contains x-mcp-header outside a properties path", strings.Join(append(path, name), "."))
						}
						continue
					}
					if err := visitProperty(childSchema, append(path, name), depth+1); err != nil {
						return err
					}
				}
				continue
			}
			found, err := containsAnnotation(child, depth+1)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("property %q contains x-mcp-header outside a properties path", strings.Join(path, "."))
			}
		}
		return nil
	}

	for name, raw := range schema.Properties {
		property, ok := raw.(map[string]any)
		if !ok {
			found, err := containsAnnotation(raw, 1)
			if err != nil {
				return nil, err
			}
			if found {
				return nil, fmt.Errorf("property %q contains x-mcp-header outside a properties path", name)
			}
			continue
		}
		if err := visitProperty(property, []string{name}, 1); err != nil {
			return nil, err
		}
	}
	if found, err := containsAnnotation(schema.Extras, 1); err != nil {
		return nil, err
	} else if found {
		return nil, errors.New("x-mcp-header is not statically reachable through properties")
	}
	return bindings, nil
}

// IsHTTPToken reports whether value contains only the RFC 9110 tchar set.
// HTTP routing and schema annotations share this security-sensitive rule.
func IsHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char > 0x7f || !(char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || strings.ContainsRune("!#$%&'*+-.^_`|~", char)) {
			return false
		}
	}
	return true
}
