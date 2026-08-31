package shared

import (
	"context"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
)

// MaxJSONRPCFrameBytes is the common inbound frame limit for HTTP and stdio.
const MaxJSONRPCFrameBytes = 1 << 20

// ResourceCatalog supplies method data while shared dispatch owns MCP parsing,
// pagination, envelopes, and errors.
type ResourceCatalog interface {
	ListResources() []map[string]any
	ListResourceTemplates() []map[string]any
	ReadResource(string) ([]map[string]any, error)
}

// MultiRoundTripResourceCatalog optionally handles input-required resource
// reads while retaining the ordinary ResourceCatalog behavior.
type MultiRoundTripResourceCatalog interface {
	ResourceCatalog
	ReadResourceRoundTrip(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error)
}

// DispatchContext carries request-scoped state shared by HTTP and stdio.
type DispatchContext struct {
	Context           context.Context
	RequestMeta       mcpv20260728.RequestMeta
	RequestStateCodec *mcpv20260728.RequestStateCodec
	PrincipalID       string
}

// CompletionRequest is the validated completion request passed to a provider.
type CompletionRequest struct {
	RefType          string
	Name             string
	URI              string
	ArgumentName     string
	ArgumentValue    string
	ContextArguments map[string]string
}

// CompletionProvider supplies candidates for an explicitly enabled catalog.
type CompletionProvider interface {
	Complete(CompletionRequest) ([]string, int, bool, error)
}

// DispatchProviders install optional resource and completion capabilities.
// Production embeddings and the isolated conformance fixture use the same seam.
type DispatchProviders struct {
	Resources  ResourceCatalog
	Completion CompletionProvider
}

// DispatchHook is an opt-in, transport-neutral JSON-RPC surface. Production
// dispatch has no hook; the conformance fixture uses one without changing the
// production catalog.
type DispatchHook func(context.Context, jsonrpc.Request, mcpv20260728.RequestMeta) (any, bool)

// DispatchWithHook invokes a transport-neutral hook when one is installed.
func DispatchWithHook(ctx context.Context, request jsonrpc.Request, meta mcpv20260728.RequestMeta, hook DispatchHook) (any, bool) {
	if hook == nil {
		return nil, false
	}
	return hook(ctx, request, meta)
}

// BuildProtocolCompleteResponse normalizes the common complete-result fields.
func BuildProtocolCompleteResponse(id any, result mcp.CompleteResult) *jsonrpc.Response {
	if result.Content == nil {
		result.Content = []any{}
	}
	normalized, err := mcpv20260728.NormalizeMethodCompleteResult("tools/call", result, resultMeta())
	if err != nil {
		return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrInternalError), "Invalid tool result", nil)
	}
	return jsonrpc.NewResponse(id, normalized)
}
