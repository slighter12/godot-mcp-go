package mcpv20260728

import (
	"encoding/json"
	"errors"
	"math"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const maxElicitationProperties = 128

var commonElicitationKeywords = map[string]struct{}{
	"type": {}, "title": {}, "description": {}, "default": {},
}

func validateElicitationRequestParams(params map[string]any) error {
	message, ok := params["message"].(string)
	if !ok || strings.TrimSpace(message) == "" {
		return errors.New("elicitation message is required")
	}
	mode, _ := params["mode"].(string)
	if mode == "url" {
		rawURL, ok := params["url"].(string)
		parsed, err := url.Parse(rawURL)
		if !ok || err != nil || !parsed.IsAbs() {
			return errors.New("elicitation URL is invalid")
		}
		return nil
	}
	if mode != "" && mode != "form" {
		return errors.New("elicitation mode is invalid")
	}
	schema, ok := params["requestedSchema"].(map[string]any)
	if !ok {
		return errors.New("elicitation requestedSchema is required")
	}
	return validateElicitationSchema(schema)
}

func validateElicitationSchema(schema map[string]any) error {
	if !onlyKeywords(schema, "type", "properties", "required", "$schema") || schema["type"] != "object" {
		return errors.New("unsupported elicitation schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) > maxElicitationProperties {
		return errors.New("elicitation properties are invalid")
	}
	for name, raw := range properties {
		if strings.TrimSpace(name) == "" {
			return errors.New("elicitation property name is invalid")
		}
		property, ok := raw.(map[string]any)
		if !ok || validateElicitationPropertySchema(property) != nil {
			return errors.New("unsupported elicitation property schema")
		}
	}
	var required []string
	if rawRequired, present := schema["required"]; present {
		var ok bool
		required, ok = schemaStringArray(rawRequired)
		if !ok {
			return errors.New("elicitation required list is invalid")
		}
	}
	seen := map[string]struct{}{}
	for _, name := range required {
		if _, exists := properties[name]; !exists {
			return errors.New("elicitation required property is unknown")
		}
		if _, duplicate := seen[name]; duplicate {
			return errors.New("elicitation required property is duplicated")
		}
		seen[name] = struct{}{}
	}
	if schemaURI, present := schema["$schema"]; present {
		if _, ok := schemaURI.(string); !ok {
			return errors.New("elicitation $schema is invalid")
		}
	}
	return nil
}

func validateElicitationPropertySchema(schema map[string]any) error {
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "boolean":
		if !onlyKeywordSet(schema, commonElicitationKeywords) || !optionalTyped(schema, "default", "boolean") {
			return errors.New("invalid boolean schema")
		}
	case "integer", "number":
		allowed := withKeywords(commonElicitationKeywords, "minimum", "maximum")
		if !onlyKeywordSet(schema, allowed) || !optionalNumber(schema, "default") || !optionalNumber(schema, "minimum") || !optionalNumber(schema, "maximum") {
			return errors.New("invalid number schema")
		}
		if typeName == "integer" && (!optionalInteger(schema, "default") || !optionalInteger(schema, "minimum") || !optionalInteger(schema, "maximum")) {
			return errors.New("invalid integer schema")
		}
		minimum, hasMin := numberValue(schema["minimum"])
		maximum, hasMax := numberValue(schema["maximum"])
		if hasMin && hasMax && minimum > maximum {
			return errors.New("invalid number range")
		}
	case "string":
		if _, enum := schema["enum"]; enum {
			return validateUntitledSingleEnumSchema(schema)
		}
		if _, oneOf := schema["oneOf"]; oneOf {
			return validateTitledSingleEnumSchema(schema)
		}
		allowed := withKeywords(commonElicitationKeywords, "minLength", "maxLength", "format")
		if !onlyKeywordSet(schema, allowed) || !optionalTyped(schema, "default", "string") || !optionalNonNegativeInteger(schema, "minLength") || !optionalNonNegativeInteger(schema, "maxLength") {
			return errors.New("invalid string schema")
		}
		if format, present := schema["format"]; present {
			value, ok := format.(string)
			if !ok || value != "date" && value != "date-time" && value != "email" && value != "uri" {
				return errors.New("invalid string format")
			}
		}
		minimum, hasMin := integerValue(schema["minLength"])
		maximum, hasMax := integerValue(schema["maxLength"])
		if hasMin && hasMax && minimum > maximum {
			return errors.New("invalid string length range")
		}
	case "array":
		return validateMultiEnumSchema(schema)
	default:
		return errors.New("unsupported elicitation type")
	}
	return validateCommonAnnotations(schema)
}

func validateUntitledSingleEnumSchema(schema map[string]any) error {
	allowed := withKeywords(commonElicitationKeywords, "enum", "enumNames")
	if !onlyKeywordSet(schema, allowed) {
		return errors.New("invalid enum schema")
	}
	values, ok := schemaStringArray(schema["enum"])
	if !ok {
		return errors.New("invalid enum values")
	}
	if names, present := schema["enumNames"]; present {
		titles, ok := schemaStringArray(names)
		if !ok || len(titles) != len(values) {
			return errors.New("invalid enum names")
		}
	}
	if defaultValue, present := schema["default"]; present && !containsString(values, defaultValue) {
		return errors.New("invalid enum default")
	}
	return validateCommonAnnotations(schema)
}

func validateTitledSingleEnumSchema(schema map[string]any) error {
	allowed := withKeywords(commonElicitationKeywords, "oneOf")
	if !onlyKeywordSet(schema, allowed) {
		return errors.New("invalid titled enum schema")
	}
	values, ok := titledEnumValues(schema["oneOf"])
	if !ok {
		return errors.New("invalid titled enum values")
	}
	if defaultValue, present := schema["default"]; present && !containsString(values, defaultValue) {
		return errors.New("invalid titled enum default")
	}
	return validateCommonAnnotations(schema)
}

func validateMultiEnumSchema(schema map[string]any) error {
	allowed := withKeywords(commonElicitationKeywords, "items", "minItems", "maxItems")
	if !onlyKeywordSet(schema, allowed) || !optionalNonNegativeInteger(schema, "minItems") || !optionalNonNegativeInteger(schema, "maxItems") {
		return errors.New("invalid multi-select schema")
	}
	items, ok := schema["items"].(map[string]any)
	if !ok || !onlyKeywords(items, "type", "enum", "anyOf") || items["type"] != "string" && items["type"] != nil {
		return errors.New("invalid multi-select items")
	}
	var values []string
	if raw, present := items["enum"]; present {
		if items["type"] != "string" {
			return errors.New("invalid multi-select item type")
		}
		values, ok = schemaStringArray(raw)
	} else if raw, present := items["anyOf"]; present {
		values, ok = titledEnumValues(raw)
	} else {
		ok = false
	}
	if !ok {
		return errors.New("invalid multi-select values")
	}
	minimum, hasMin := integerValue(schema["minItems"])
	maximum, hasMax := integerValue(schema["maxItems"])
	if hasMin && hasMax && minimum > maximum {
		return errors.New("invalid multi-select range")
	}
	if rawDefault, present := schema["default"]; present {
		defaults, ok := schemaStringArray(rawDefault)
		if !ok || !allStringsIn(defaults, values) {
			return errors.New("invalid multi-select default")
		}
	}
	return validateCommonAnnotations(schema)
}

func validateElicitationContent(content, schema map[string]any) bool {
	if validateElicitationSchema(schema) != nil {
		return false
	}
	properties := schema["properties"].(map[string]any)
	required, _ := schemaStringArray(schema["required"])
	for _, name := range required {
		if _, present := content[name]; !present {
			return false
		}
	}
	for name, value := range content {
		rawSchema, known := properties[name]
		if !known {
			continue
		}
		if !matchesElicitationProperty(value, rawSchema.(map[string]any)) {
			return false
		}
	}
	return true
}

func matchesElicitationProperty(value any, schema map[string]any) bool {
	switch schema["type"] {
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer", "number":
		number, ok := numberValue(value)
		if !ok || schema["type"] == "integer" && math.Trunc(number) != number {
			return false
		}
		minimum, hasMin := numberValue(schema["minimum"])
		maximum, hasMax := numberValue(schema["maximum"])
		return (!hasMin || number >= minimum) && (!hasMax || number <= maximum)
	case "string":
		text, ok := value.(string)
		if !ok {
			return false
		}
		if raw, present := schema["enum"]; present {
			values, _ := schemaStringArray(raw)
			return containsString(values, text)
		}
		if raw, present := schema["oneOf"]; present {
			values, _ := titledEnumValues(raw)
			return containsString(values, text)
		}
		length := int64(utf8.RuneCountInString(text))
		minimum, hasMin := integerValue(schema["minLength"])
		maximum, hasMax := integerValue(schema["maxLength"])
		return (!hasMin || length >= minimum) && (!hasMax || length <= maximum) && matchesStringFormat(text, schema["format"])
	case "array":
		items, ok := value.([]any)
		if !ok {
			return false
		}
		itemSchema := schema["items"].(map[string]any)
		values, ok := schemaStringArray(itemSchema["enum"])
		if !ok {
			values, _ = titledEnumValues(itemSchema["anyOf"])
		}
		selected := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok || !containsString(values, text) {
				return false
			}
			selected = append(selected, text)
		}
		minimum, hasMin := integerValue(schema["minItems"])
		maximum, hasMax := integerValue(schema["maxItems"])
		return (!hasMin || int64(len(selected)) >= minimum) && (!hasMax || int64(len(selected)) <= maximum)
	default:
		return false
	}
}

func matchesStringFormat(value string, raw any) bool {
	format, present := raw.(string)
	if !present || format == "" {
		return true
	}
	switch format {
	case "date":
		_, err := time.Parse("2006-01-02", value)
		return err == nil
	case "date-time":
		_, err := time.Parse(time.RFC3339, value)
		return err == nil
	case "email":
		address, err := mail.ParseAddress(value)
		return err == nil && address.Address == value
	case "uri":
		parsed, err := url.Parse(value)
		return err == nil && parsed.IsAbs()
	default:
		return false
	}
}

func validateCommonAnnotations(schema map[string]any) error {
	for _, key := range []string{"title", "description"} {
		if value, present := schema[key]; present {
			if _, ok := value.(string); !ok {
				return errors.New("invalid schema annotation")
			}
		}
	}
	return nil
}

func onlyKeywords(value map[string]any, allowed ...string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		set[key] = struct{}{}
	}
	return onlyKeywordSet(value, set)
}

func onlyKeywordSet(value map[string]any, allowed map[string]struct{}) bool {
	for key := range value {
		if _, ok := allowed[key]; !ok {
			return false
		}
	}
	return true
}

func withKeywords(base map[string]struct{}, extra ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(base)+len(extra))
	for key := range base {
		result[key] = struct{}{}
	}
	for _, key := range extra {
		result[key] = struct{}{}
	}
	return result
}

func optionalTyped(schema map[string]any, key, kind string) bool {
	value, present := schema[key]
	if !present {
		return true
	}
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	default:
		return false
	}
}

func optionalNumber(schema map[string]any, key string) bool {
	value, present := schema[key]
	if !present {
		return true
	}
	_, ok := numberValue(value)
	return ok
}

func optionalNonNegativeInteger(schema map[string]any, key string) bool {
	value, present := schema[key]
	if !present {
		return true
	}
	integer, ok := integerValue(value)
	return ok && integer >= 0
}

func optionalInteger(schema map[string]any, key string) bool {
	value, present := schema[key]
	if !present {
		return true
	}
	_, ok := integerValue(value)
	return ok
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
	case float64:
		return number, !math.IsInf(number, 0) && !math.IsNaN(number)
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func integerValue(value any) (int64, bool) {
	number, ok := numberValue(value)
	if !ok || math.Trunc(number) != number || number < math.MinInt64 || number > math.MaxInt64 {
		return 0, false
	}
	return int64(number), true
}

func schemaStringArray(value any) ([]string, bool) {
	if value == nil {
		return nil, false
	}
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return strings, true
		}
		return nil, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func titledEnumValues(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		option, ok := item.(map[string]any)
		if !ok || !onlyKeywords(option, "const", "title") {
			return nil, false
		}
		constant, constantOK := option["const"].(string)
		_, titleOK := option["title"].(string)
		if !constantOK || !titleOK {
			return nil, false
		}
		values = append(values, constant)
	}
	return values, true
}

func containsString(values []string, wanted any) bool {
	text, ok := wanted.(string)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == text {
			return true
		}
	}
	return false
}

func allStringsIn(values, allowed []string) bool {
	for _, value := range values {
		if !containsString(allowed, value) {
			return false
		}
	}
	return true
}
