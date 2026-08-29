package http

import (
	"fmt"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

func (s *Server) handleGodotResource(path string) (any, error) {
	switch path {
	case "godot://script/current":
		return map[string]any{"type": "script", "path": "current"}, nil
	case "godot://scene/current":
		return map[string]any{"type": "scene", "path": "current"}, nil
	case "godot://project/info":
		return map[string]any{"name": "godot-mcp", "version": mcp.ServerVersion, "type": "godot"}, nil
	case "godot://policy/godot-checks":
		return map[string]any{"policy": "policy-godot", "checks": promptcatalog.GodotPolicyChecks()}, nil
	case "godot://runtime/metrics":
		return runtimebridge.HealthSnapshot(time.Now().UTC()), nil
	default:
		return nil, fmt.Errorf("unknown resource path: %s", path)
	}
}
