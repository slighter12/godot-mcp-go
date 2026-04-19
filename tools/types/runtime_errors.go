package types

import "maps"

func NewRuntimeNotAvailableError(message string, tool string, code string, data map[string]any) *SemanticError {
	payload := map[string]any{}
	maps.Copy(payload, data)
	payload["feature"] = "runtime"
	payload["tool"] = tool
	payload["code"] = code
	return NewNotAvailableError(message, payload)
}

func NewRuntimeInvalidParamsError(message string, tool string, code string, data map[string]any) *SemanticError {
	payload := map[string]any{}
	maps.Copy(payload, data)
	payload["feature"] = "runtime"
	payload["tool"] = tool
	payload["code"] = code
	return NewSemanticError(SemanticKindInvalidParams, message, payload)
}
