package types

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

type RuntimeCommandValidateFunc func(map[string]any, string) (map[string]any, error)
type RuntimeCommandInvalidJSONErrorBuilder func(error) error
type RuntimeCommandProgressNotifier func(RuntimeCommandProgressEvent)

type RuntimeCommandProgressEvent struct {
	SessionID     string
	CommandName   string
	Progress      float64
	Message       string
	ProgressToken any
}

type RuntimeCommandDispatchOptions struct {
	RawArgs                  json.RawMessage
	CommandName              string
	Timeout                  time.Duration
	SessionRequiredMessage   string
	BridgeUnavailableMessage string
	InvalidJSONError         RuntimeCommandInvalidJSONErrorBuilder
	Validate                 RuntimeCommandValidateFunc
}

var (
	runtimeCommandProgressNotifierMu sync.RWMutex
	runtimeCommandProgressNotifier   RuntimeCommandProgressNotifier
)

func SetRuntimeCommandProgressNotifier(notifier RuntimeCommandProgressNotifier) {
	runtimeCommandProgressNotifierMu.Lock()
	defer runtimeCommandProgressNotifierMu.Unlock()
	runtimeCommandProgressNotifier = notifier
}

func DispatchRuntimeCommand(options RuntimeCommandDispatchOptions) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(options.RawArgs, &arguments); err != nil {
		if options.InvalidJSONError != nil {
			return nil, options.InvalidJSONError(err)
		}
		return nil, err
	}

	ctx := ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, NewNotAvailableError(options.SessionRequiredMessage, map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    options.CommandName,
		})
	}

	commandArgs := StripMCPContext(arguments)
	var err error
	if options.Validate != nil {
		commandArgs, err = options.Validate(commandArgs, options.CommandName)
		if err != nil {
			return nil, err
		}
	}

	runtimeSessionID := ctx.EffectiveRuntimeCommandSessionID()
	if strings.TrimSpace(runtimeSessionID) == "" {
		return nil, NewNotAvailableError(options.BridgeUnavailableMessage, map[string]any{
			"feature": "runtime_bridge",
			"reason":  "runtime_session_missing",
			"tool":    options.CommandName,
		})
	}

	emitRuntimeCommandProgress(ctx, options.CommandName, 0.4, "dispatching runtime command")
	ack, ok, reason := runtimebridge.DefaultCommandBroker().DispatchAndWait(runtimeSessionID, options.CommandName, commandArgs, options.Timeout)
	if !ok {
		emitRuntimeCommandProgress(ctx, options.CommandName, 1.0, "runtime command unavailable")
		return nil, NewNotAvailableError(options.BridgeUnavailableMessage, map[string]any{
			"feature": "runtime_bridge",
			"reason":  reason,
			"tool":    options.CommandName,
		})
	}
	emitRuntimeCommandProgress(ctx, options.CommandName, 1.0, "runtime command acknowledged")
	return json.Marshal(RuntimeCommandAckEnvelope(ack))
}

func RuntimeCommandAckEnvelope(ack runtimebridge.CommandAck) map[string]any {
	result := map[string]any{
		"success":         ack.Success,
		"command_id":      ack.CommandID,
		"result":          ack.Result,
		"error":           ack.Error,
		"acknowledged_at": ack.AckedAt.UTC().Format(time.RFC3339Nano),
	}
	if schemaVersion, ok := ack.SchemaVersion(); ok {
		result["schema_version"] = schemaVersion
	}
	if reason, ok := ack.Reason(); ok {
		result["reason"] = reason
	}
	if retryable, ok := ack.Retryable(); ok {
		result["retryable"] = retryable
	}
	return result
}

func emitRuntimeCommandProgress(ctx MCPContext, commandName string, progress float64, message string) {
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return
	}
	if !ctx.EmitProgress {
		return
	}

	runtimeCommandProgressNotifierMu.RLock()
	notifier := runtimeCommandProgressNotifier
	runtimeCommandProgressNotifierMu.RUnlock()
	if notifier == nil {
		return
	}

	notifier(RuntimeCommandProgressEvent{
		SessionID:     ctx.SessionID,
		CommandName:   commandName,
		Progress:      progress,
		Message:       message,
		ProgressToken: ctx.ProgressToken,
	})
}
