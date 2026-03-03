package toolspec

import (
	"regexp"
	"slices"
	"strings"
)

const (
	ToolPermissionAllowAll  = "allow_all"
	ToolPermissionReadOnly  = "read_only"
	ToolPermissionAllowList = "allow_list"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

var readOnlyToolNames = map[string]struct{}{
	"godot.offerings.list":         {},
	"godot.runtime.health.get":     {},
	"godot.project.settings.get":   {},
	"godot.project.resources.list": {},
	"godot.editor.state.get":       {},
	"godot.node.tree.get":          {},
	"godot.node.properties.get":    {},
	"godot.scene.list":             {},
	"godot.scene.read":             {},
	"godot.script.list":            {},
	"godot.script.read":            {},
	"godot.script.analyze":         {},
	"godot.runtime.ping":           {},
}

var mutatingToolNames = map[string]struct{}{
	"godot.project.run":   {},
	"godot.project.stop":  {},
	"godot.scene.create":  {},
	"godot.scene.save":    {},
	"godot.scene.apply":   {},
	"godot.node.create":   {},
	"godot.node.delete":   {},
	"godot.node.modify":   {},
	"godot.script.create": {},
	"godot.script.modify": {},
}

var internalBridgeToolNames = map[string]struct{}{
	"godot.runtime.sync": {},
	"godot.runtime.ping": {},
	"godot.runtime.ack":  {},
}

func ValidateToolName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	if !toolNamePattern.MatchString(trimmed) {
		return false
	}
	segments := strings.Split(trimmed, ".")
	if len(segments) < 3 || len(segments) > 4 {
		return false
	}
	if segments[0] != "godot" {
		return false
	}
	for _, segment := range segments {
		if strings.TrimSpace(segment) == "" {
			return false
		}
	}
	return true
}

func IsReadOnlyTool(name string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "godot://") {
		return true
	}
	_, ok := readOnlyToolNames[trimmed]
	return ok
}

func IsMutatingTool(name string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return false
	}
	_, ok := mutatingToolNames[trimmed]
	return ok
}

func IsToolAllowed(name string, permissionMode string, allowList []string) bool {
	trimmedName := strings.ToLower(strings.TrimSpace(name))
	if IsInternalBridgeTool(trimmedName) {
		return true
	}
	mode := strings.ToLower(strings.TrimSpace(permissionMode))

	switch mode {
	case "", ToolPermissionAllowAll:
		return true
	case ToolPermissionReadOnly:
		return IsReadOnlyTool(trimmedName)
	case ToolPermissionAllowList:
		normalizedAllowList := make([]string, 0, len(allowList))
		for _, candidate := range allowList {
			normalizedAllowList = append(normalizedAllowList, strings.ToLower(strings.TrimSpace(candidate)))
		}
		return slices.Contains(normalizedAllowList, trimmedName)
	default:
		return false
	}
}

func IsInternalBridgeTool(name string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return false
	}
	_, ok := internalBridgeToolNames[trimmed]
	return ok
}
