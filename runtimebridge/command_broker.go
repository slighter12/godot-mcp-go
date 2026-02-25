package runtimebridge

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultCommandTimeout = 8 * time.Second

var (
	defaultCommandBroker = NewCommandBroker(defaultCommandTimeout)
	commandSeq           atomic.Uint64
)

// CommandAck stores plugin execution acknowledgement for one command.
type CommandAck struct {
	CommandID string
	Success   bool
	Result    map[string]any
	Error     string
	AckedAt   time.Time
}

func (a CommandAck) SchemaVersion() (string, bool) {
	if a.Result == nil {
		return "", false
	}
	value, ok := a.Result["schema_version"]
	if !ok {
		return "", false
	}
	schemaVersion, ok := value.(string)
	if !ok {
		return "", false
	}
	schemaVersion = strings.TrimSpace(schemaVersion)
	if schemaVersion == "" {
		return "", false
	}
	return schemaVersion, true
}

func (a CommandAck) Reason() (string, bool) {
	if a.Result == nil {
		return "", false
	}
	value, ok := a.Result["reason"]
	if !ok {
		return "", false
	}
	reason, ok := value.(string)
	if !ok {
		return "", false
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", false
	}
	return reason, true
}

func (a CommandAck) Retryable() (bool, bool) {
	if a.Result == nil {
		return false, false
	}
	value, ok := a.Result["retryable"]
	if !ok {
		return false, false
	}
	retryable, ok := value.(bool)
	if !ok {
		return false, false
	}
	return retryable, true
}

type pendingCommand struct {
	sessionID string
	resultCh  chan CommandAck
}

// CommandBroker coordinates server->plugin command round trips.
type CommandBroker struct {
	mu             sync.Mutex
	defaultTimeout time.Duration
	pending        map[string]pendingCommand
}

func NewCommandBroker(defaultTimeout time.Duration) *CommandBroker {
	if defaultTimeout <= 0 {
		defaultTimeout = defaultCommandTimeout
	}
	return &CommandBroker{
		defaultTimeout: defaultTimeout,
		pending:        make(map[string]pendingCommand),
	}
}

func DefaultCommandBroker() *CommandBroker {
	return defaultCommandBroker
}

func ResetDefaultCommandBrokerForTests(defaultTimeout time.Duration) {
	defaultCommandBroker = NewCommandBroker(defaultTimeout)
}

func (b *CommandBroker) DispatchAndWait(sessionID string, commandName string, arguments map[string]any, timeout time.Duration) (CommandAck, bool, string) {
	if b == nil {
		return CommandAck{}, false, "command_broker_unavailable"
	}
	if strings.TrimSpace(sessionID) == "" {
		return CommandAck{}, false, "session_missing"
	}
	if strings.TrimSpace(commandName) == "" {
		return CommandAck{}, false, "command_name_missing"
	}
	if timeout <= 0 {
		timeout = b.defaultTimeout
	}
	if arguments == nil {
		arguments = map[string]any{}
	}

	commandID := nextCommandID()
	waiter := pendingCommand{
		sessionID: sessionID,
		resultCh:  make(chan CommandAck, 1),
	}

	b.mu.Lock()
	b.pending[commandID] = waiter
	b.mu.Unlock()

	message := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/godot/command",
		"params": map[string]any{
			"command_id": commandID,
			"commandId":  commandID,
			"name":       commandName,
			"arguments":  arguments,
		},
	}
	if !sendToSession(sessionID, message) {
		b.remove(commandID)
		return CommandAck{}, false, "command_transport_unavailable"
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ack := <-waiter.resultCh:
		if ack.AckedAt.IsZero() {
			ack.AckedAt = time.Now().UTC()
		}
		return ack, true, ""
	case <-timer.C:
		b.remove(commandID)
		return CommandAck{}, false, "command_ack_timeout"
	}
}

func (b *CommandBroker) Ack(sessionID string, ack CommandAck) bool {
	if b == nil {
		return false
	}
	commandID := strings.TrimSpace(ack.CommandID)
	if commandID == "" {
		return false
	}

	b.mu.Lock()
	pending, exists := b.pending[commandID]
	if !exists || pending.sessionID != sessionID {
		b.mu.Unlock()
		return false
	}
	delete(b.pending, commandID)
	b.mu.Unlock()

	if ack.Result == nil {
		ack.Result = map[string]any{}
	} else {
		ack.Result = maps.Clone(ack.Result)
	}
	if ack.AckedAt.IsZero() {
		ack.AckedAt = time.Now().UTC()
	}

	select {
	case pending.resultCh <- ack:
		return true
	default:
		return false
	}
}

func (b *CommandBroker) remove(commandID string) {
	b.mu.Lock()
	delete(b.pending, commandID)
	b.mu.Unlock()
}

func nextCommandID() string {
	seq := commandSeq.Add(1)
	return fmt.Sprintf("cmd_%d_%s", time.Now().UTC().UnixNano(), strconv.FormatUint(seq, 10))
}
