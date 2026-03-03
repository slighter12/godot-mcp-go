package notifications

import (
	"strings"
)

func IsValidProgressToken(token any) bool {
	switch value := token.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case float64:
		return true
	default:
		return false
	}
}

func ProgressParams(progressToken any, progress float64, message string) map[string]any {
	params := map[string]any{
		"progressToken": progressToken,
		"progress":      progress,
		"total":         1.0,
	}
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		params["message"] = trimmed
	}
	return params
}
