package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

type stdioCompletionProviderFunc func(shared.CompletionRequest) ([]string, int, bool, error)

func (f stdioCompletionProviderFunc) Complete(request shared.CompletionRequest) ([]string, int, bool, error) {
	return f(request)
}

func TestMain(m *testing.M) {
	_ = logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON, "logs/stdio.log")
	os.Exit(m.Run())
}

func TestServeRejectsOneOversizedFrameThenTerminates(t *testing.T) {
	server := NewStdioServer(tools.NewManager())
	var output bytes.Buffer
	server.output = &output
	input := strings.NewReader(strings.Repeat(" ", shared.MaxJSONRPCFrameBytes+1) + "\n" + `{"jsonrpc":"2.0","id":"later","method":"server/discover"}` + "\n")

	err := server.serve(input)
	if err == nil || !strings.Contains(err.Error(), "request body too large") {
		t.Fatalf("oversized frame did not terminate stdio: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one response before termination, got %d: %s", len(lines), output.String())
	}
	var response jsonrpc.Response
	if decodeErr := json.Unmarshal([]byte(lines[0]), &response); decodeErr != nil || response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidRequest) || response.Error.Message != "Request body too large" {
		t.Fatalf("unexpected oversized-frame response: decode=%v response=%#v", decodeErr, response)
	}
}

func TestReadStdioFrameExcludesNewlineDelimiterFromLimit(t *testing.T) {
	payload := strings.Repeat(" ", shared.MaxJSONRPCFrameBytes)
	frame, oversized, err := readStdioFrame(bufio.NewReader(strings.NewReader(payload + "\n")))
	if err != nil || oversized || strings.TrimSuffix(string(frame), "\n") != payload {
		t.Fatalf("limit-sized frame was rejected: oversized=%t err=%v bytes=%d", oversized, err, len(frame))
	}
}

func TestServeTerminatesOversizedHeldOpenFrameWithoutWaitingForNewline(t *testing.T) {
	server := NewStdioServer(tools.NewManager())
	var output bytes.Buffer
	server.output = &output
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.serve(reader)
	}()
	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.Write(bytes.Repeat([]byte{'x'}, shared.MaxJSONRPCFrameBytes+1))
		writeDone <- err
	}()

	select {
	case err := <-serveDone:
		if err == nil || !strings.Contains(err.Error(), "request body too large") {
			t.Fatalf("unexpected serve result: %v", err)
		}
	case <-time.After(2 * time.Second):
		_ = writer.CloseWithError(errors.New("test timeout"))
		t.Fatal("stdio server waited for newline after detecting oversized frame")
	}
	select {
	case <-writeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("held-open writer did not unblock after stdio termination")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one oversized response, got %d: %s", len(lines), output.String())
	}
	var response jsonrpc.Response
	if err := json.Unmarshal([]byte(lines[0]), &response); err != nil || response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidRequest) {
		t.Fatalf("unexpected oversized response: decode=%v response=%#v", err, response)
	}
}

func TestModernStdioDispatchesDiscoverToolsAndCalls(t *testing.T) {
	server := newTestStdioServer(true)
	options := modernStdioClientOptions{EditorSessionID: "editor-stdio"}

	discover := dispatchModernTest(t, server, "discover", "server/discover", map[string]any{}, options)
	if discover.Error != nil {
		t.Fatalf("server/discover error: %#v", discover.Error)
	}
	if mustMap(t, discover.Result)["resultType"] != "complete" {
		t.Fatalf("unexpected discovery result: %#v", discover.Result)
	}

	list := dispatchModernTest(t, server, "tools", "tools/list", map[string]any{}, options)
	if list.Error != nil || mustMap(t, list.Result)["resultType"] != "complete" {
		t.Fatalf("unexpected tools/list response: %#v", list)
	}

	call := dispatchModernTest(t, server, "health", "tools/call", map[string]any{
		"name":      "godot.runtime.health.get",
		"arguments": map[string]any{},
	}, options)
	if call.Error != nil {
		t.Fatalf("tools/call error: %#v", call.Error)
	}
	result := mustMap(t, call.Result)
	if result["resultType"] != "complete" || result["isError"] != false {
		t.Fatalf("unexpected tools/call result: %#v", result)
	}
}

func TestModernStdioFramesTemplatesAndDoesNotAdvertiseUnavailableCompletion(t *testing.T) {
	server := newTestStdioServer(true)
	templates := dispatchModernTest(t, server, "templates", "resources/templates/list", map[string]any{}, modernStdioClientOptions{})
	if templates.Error != nil {
		t.Fatalf("resources/templates/list error: %#v", templates.Error)
	}
	if result := mustMap(t, templates.Result); result["resultType"] != "complete" || result["cacheScope"] != "public" {
		t.Fatalf("unexpected templates result: %#v", result)
	}

	completion := dispatchModernTest(t, server, "completion", "completion/complete", map[string]any{
		"ref":      map[string]any{"type": "ref/prompt", "name": "stdio-completion"},
		"argument": map[string]any{"name": "value", "value": ""},
	}, modernStdioClientOptions{})
	if completion.Error == nil || completion.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected unavailable completion to return method not found: %#v", completion)
	}
}

func TestModernStdioUsesConfiguredCompletionProvider(t *testing.T) {
	server := newTestStdioServer(true)
	server.AttachDispatchProviders(shared.DispatchProviders{Completion: stdioCompletionProviderFunc(func(shared.CompletionRequest) ([]string, int, bool, error) {
		return []string{"res://Main.tscn"}, 1, false, nil
	})})

	discover := dispatchModernTest(t, server, "discover-completion", "server/discover", map[string]any{}, modernStdioClientOptions{})
	if _, ok := mustMap(t, mustMap(t, discover.Result)["capabilities"])["completions"]; !ok {
		t.Fatalf("configured stdio server did not advertise completions: %#v", discover.Result)
	}
	response := dispatchModernTest(t, server, "completion", "completion/complete", map[string]any{
		"ref": map[string]any{"type": "ref/prompt", "name": "scene"}, "argument": map[string]any{"name": "path", "value": "res://"},
	}, modernStdioClientOptions{})
	if response.Error != nil {
		t.Fatalf("configured stdio completion failed: %#v", response.Error)
	}
	completion := mustMap(t, mustMap(t, response.Result)["completion"])
	values, ok := completion["values"].([]string)
	if !ok || len(values) != 1 || values[0] != "res://Main.tscn" {
		t.Fatalf("unexpected completion result: %#v", completion)
	}
}

func TestModernStdioRemovedMethodsReturnDiagnostics(t *testing.T) {
	server := newTestStdioServer(true)
	response := dispatchModernTest(t, server, 1, "initialize", map[string]any{}, modernStdioClientOptions{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected method-not-found error, got %#v", response)
	}
	data := mustMap(t, response.Error.Data)
	if supported, ok := data["supported"].([]string); !ok || len(supported) != 1 || supported[0] != mcpv20260728.ProtocolVersion {
		t.Fatalf("expected supported version diagnostic, got %#v", data["supported"])
	}
}

func TestModernStdioInvalidMetadataAndUnknownMethod(t *testing.T) {
	server := newTestStdioServer(true)
	response := protocolErrorResponse(1, "initialize", mcpv20260728.ErrMissingClientCapabilities)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid metadata response, got %#v", response)
	}
	data := mustMap(t, response.Error.Data)
	supported, ok := data["supported"].([]string)
	if !ok || len(supported) != 1 || supported[0] != mcpv20260728.ProtocolVersion {
		t.Fatalf("expected supported version diagnostic, got %#v", data)
	}

	unknown := dispatchModernTest(t, server, "unknown", "not/implemented", map[string]any{}, modernStdioClientOptions{})
	if unknown.Error == nil || unknown.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected unknown method error, got %#v", unknown)
	}
}

func TestModernStdioDuplicateSubscriptionIDsRemainIndependent(t *testing.T) {
	server := newTestStdioServer(false)
	server.output = io.Discard
	request := jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  "subscriptions/listen",
		Params: mustRaw(t, modernStdioParams(map[string]any{
			"notifications": map[string]any{"toolsListChanged": true},
		}, modernStdioClientOptions{})),
	}
	meta, err := mcpv20260728.ParseRequestMeta(request.Params)
	if err != nil {
		t.Fatalf("parse subscription metadata: %v", err)
	}

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	doneA := make(chan struct{})
	doneB := make(chan struct{})
	go func() {
		defer close(doneA)
		server.handleSubscription(ctxA, cancelA, request, meta)
	}()
	go func() {
		defer close(doneB)
		server.handleSubscription(ctxB, cancelB, request, meta)
	}()

	waitForStdioSubscriptionCount(t, server, 2)
	server.handleCancellation(jsonrpc.Request{
		Params: mustRaw(t, map[string]any{"requestId": 1}),
	})
	waitForStdioSubscriptionCount(t, server, 0)

	select {
	case <-doneA:
	case <-time.After(time.Second):
		t.Fatal("first duplicate subscription did not stop")
	}
	select {
	case <-doneB:
	case <-time.After(time.Second):
		t.Fatal("second duplicate subscription did not stop")
	}
}

func TestModernStdioCancellationDistinguishesNumericAndStringIDs(t *testing.T) {
	server := newTestStdioServer(false)
	server.output = io.Discard
	params := mustRaw(t, modernStdioParams(map[string]any{
		"notifications": map[string]any{"toolsListChanged": true},
	}, modernStdioClientOptions{}))
	numericRequest := jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  "subscriptions/listen",
		Params:  params,
	}
	stringRequest := numericRequest
	stringRequest.ID = "1"
	meta, err := mcpv20260728.ParseRequestMeta(params)
	if err != nil {
		t.Fatalf("parse subscription metadata: %v", err)
	}

	numericCtx, numericCancel := context.WithCancel(context.Background())
	defer numericCancel()
	stringCtx, stringCancel := context.WithCancel(context.Background())
	defer stringCancel()
	numericDone := make(chan struct{})
	stringDone := make(chan struct{})
	go func() {
		defer close(numericDone)
		server.handleSubscription(numericCtx, numericCancel, numericRequest, meta)
	}()
	go func() {
		defer close(stringDone)
		server.handleSubscription(stringCtx, stringCancel, stringRequest, meta)
	}()

	waitForStdioSubscriptionCount(t, server, 2)
	server.handleCancellation(jsonrpc.Request{
		Params: mustRaw(t, map[string]any{"requestId": 1}),
	})
	select {
	case <-numericDone:
	case <-time.After(time.Second):
		t.Fatal("numeric subscription was not cancelled")
	}
	select {
	case <-stringDone:
		t.Fatal("string subscription was cancelled by numeric request ID")
	case <-time.After(50 * time.Millisecond):
	}
	waitForStdioSubscriptionCount(t, server, 1)

	server.handleCancellation(jsonrpc.Request{
		Params: mustRaw(t, map[string]any{"requestId": "1"}),
	})
	select {
	case <-stringDone:
	case <-time.After(time.Second):
		t.Fatal("string subscription was not cancelled")
	}
	waitForStdioSubscriptionCount(t, server, 0)
}

func TestModernStdioCancellationPreservesLargeIntegerIDs(t *testing.T) {
	server := newTestStdioServer(false)
	server.output = io.Discard
	requestID := json.Number("9007199254740993")
	params := mustRaw(t, modernStdioParams(map[string]any{
		"notifications": map[string]any{"toolsListChanged": true},
	}, modernStdioClientOptions{}))
	request := jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      requestID,
		Method:  "subscriptions/listen",
		Params:  params,
	}
	meta, err := mcpv20260728.ParseRequestMeta(params)
	if err != nil {
		t.Fatalf("parse subscription metadata: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.handleSubscription(ctx, cancel, request, meta)
	}()

	waitForStdioSubscriptionCount(t, server, 1)
	server.handleCancellation(jsonrpc.Request{
		Params: mustRaw(t, map[string]any{"requestId": requestID}),
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("large integer subscription was not cancelled")
	}
	waitForStdioSubscriptionCount(t, server, 0)
}

func TestRequestIDKeyPreservesJSONTypeAndPrecision(t *testing.T) {
	if requestIDKey(1) == requestIDKey("1") {
		t.Fatal("numeric and string request IDs must have distinct routing keys")
	}
	if requestIDKey(json.Number("9007199254740992")) == requestIDKey(json.Number("9007199254740993")) {
		t.Fatal("distinct large integer request IDs must have distinct routing keys")
	}
}

func waitForStdioSubscriptionCount(t *testing.T, server *StdioServer, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		server.subscriptionsMu.Lock()
		got := len(server.subscriptions)
		server.subscriptionsMu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	server.subscriptionsMu.Lock()
	got := len(server.subscriptions)
	server.subscriptionsMu.Unlock()
	t.Fatalf("expected %d active subscriptions, got %d", want, got)
}
