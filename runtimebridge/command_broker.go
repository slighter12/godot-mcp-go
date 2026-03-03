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

// CommandBrokerMetrics is a snapshot of runtime command broker observability data.
type CommandBrokerMetrics struct {
	DispatchTotal       uint64            `json:"dispatch_total"`
	AckedTotal          uint64            `json:"acked_total"`
	TimeoutTotal        uint64            `json:"timeout_total"`
	TransportErrorTotal uint64            `json:"transport_error_total"`
	FailureReasons      map[string]uint64 `json:"failure_reasons"`
	AvgLatencyMS        float64           `json:"avg_latency_ms"`
	MaxLatencyMS        int64             `json:"max_latency_ms"`
}

type commandBrokerMetricsState struct {
	mu                  sync.Mutex
	dispatchTotal       uint64
	ackedTotal          uint64
	timeoutTotal        uint64
	transportErrorTotal uint64
	totalLatencyMS      int64
	maxLatencyMS        int64
	failureReasons      map[string]uint64
}

// CommandBroker coordinates server->plugin command round trips.
type CommandBroker struct {
	mu             sync.Mutex
	defaultTimeout time.Duration
	pending        map[string]pendingCommand
	metrics        commandBrokerMetricsState
}

func NewCommandBroker(defaultTimeout time.Duration) *CommandBroker {
	if defaultTimeout <= 0 {
		defaultTimeout = defaultCommandTimeout
	}
	return &CommandBroker{
		defaultTimeout: defaultTimeout,
		pending:        make(map[string]pendingCommand),
		metrics: commandBrokerMetricsState{
			failureReasons: make(map[string]uint64),
		},
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
			"name":       commandName,
			"arguments":  arguments,
		},
	}
	startedAt := time.Now().UTC()
	b.metricsDispatch()
	if !sendToSession(sessionID, message) {
		b.remove(commandID)
		b.metricsFailure("command_transport_unavailable")
		return CommandAck{}, false, "command_transport_unavailable"
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ack := <-waiter.resultCh:
		if ack.AckedAt.IsZero() {
			ack.AckedAt = time.Now().UTC()
		}
		b.metricsAcked(ack.AckedAt.Sub(startedAt))
		return ack, true, ""
	case <-timer.C:
		b.remove(commandID)
		b.metricsFailure("command_ack_timeout")
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

func (b *CommandBroker) Metrics() CommandBrokerMetrics {
	if b == nil {
		return CommandBrokerMetrics{
			FailureReasons: map[string]uint64{},
		}
	}

	b.metrics.mu.Lock()
	defer b.metrics.mu.Unlock()

	out := CommandBrokerMetrics{
		DispatchTotal:       b.metrics.dispatchTotal,
		AckedTotal:          b.metrics.ackedTotal,
		TimeoutTotal:        b.metrics.timeoutTotal,
		TransportErrorTotal: b.metrics.transportErrorTotal,
		FailureReasons:      make(map[string]uint64, len(b.metrics.failureReasons)),
		AvgLatencyMS:        0,
		MaxLatencyMS:        b.metrics.maxLatencyMS,
	}
	for reason, count := range b.metrics.failureReasons {
		out.FailureReasons[reason] = count
	}
	if b.metrics.ackedTotal > 0 {
		out.AvgLatencyMS = float64(b.metrics.totalLatencyMS) / float64(b.metrics.ackedTotal)
	}
	return out
}

func (b *CommandBroker) metricsDispatch() {
	b.metrics.mu.Lock()
	defer b.metrics.mu.Unlock()
	b.metrics.dispatchTotal = b.metrics.dispatchTotal + 1
}

func (b *CommandBroker) metricsAcked(latency time.Duration) {
	if latency < 0 {
		latency = 0
	}
	latencyMS := latency.Milliseconds()
	b.metrics.mu.Lock()
	defer b.metrics.mu.Unlock()
	b.metrics.ackedTotal = b.metrics.ackedTotal + 1
	b.metrics.totalLatencyMS = b.metrics.totalLatencyMS + latencyMS
	if latencyMS > b.metrics.maxLatencyMS {
		b.metrics.maxLatencyMS = latencyMS
	}
}

func (b *CommandBroker) metricsFailure(reason string) {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "unknown"
	}
	b.metrics.mu.Lock()
	defer b.metrics.mu.Unlock()
	b.metrics.failureReasons[trimmedReason] = b.metrics.failureReasons[trimmedReason] + 1
	switch trimmedReason {
	case "command_transport_unavailable":
		b.metrics.transportErrorTotal = b.metrics.transportErrorTotal + 1
	case "command_ack_timeout":
		b.metrics.timeoutTotal = b.metrics.timeoutTotal + 1
	}
}
