package shared

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/slighter12/godot-mcp-go/internal/application/toolpipeline"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

const pageSize = 50
const maxRenderedPromptBytes = 128 * 1024
const toolExecutionErrorMessage = "Tool execution failed"

type promptsGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

type PromptRenderOptions struct {
	Mode                   string
	RejectUnknownArguments bool
	GovernanceRoots        []PromptGovernanceRoot
}

type PromptGovernanceRoot struct {
	Path string
	Tier string
}

type ToolCallContext struct {
	SessionID               string
	RuntimeSessionID        string
	RuntimeCommandSessionID string
	SessionInitialized      bool
	MutatingAllowed         bool
}

const (
	PromptRenderingModeLegacy   = "legacy"
	PromptRenderingModeStrict   = "strict"
	PromptRenderingModeAdvanced = "advanced"
	ToolPermissionAllowAll      = "allow_all"
	ToolPermissionReadOnly      = "read_only"
	ToolPermissionAllowList     = "allow_list"
)

var errRenderedPromptTooLarge = errors.New("rendered prompt too large")

var advancedTemplateControlTokens = map[string]struct{}{
	"if":       {},
	"else":     {},
	"end":      {},
	"range":    {},
	"with":     {},
	"template": {},
	"define":   {},
	"block":    {},
}

type ToolCallOptions struct {
	SchemaValidationEnabled   bool
	RejectUnknownArguments    bool
	PermissionMode            string
	AllowedTools              []string
	EmitProgressNotifications bool
}

func DefaultPromptRenderOptions() PromptRenderOptions {
	return PromptRenderOptions{
		Mode:                   PromptRenderingModeLegacy,
		RejectUnknownArguments: false,
	}
}

func DefaultToolCallOptions() ToolCallOptions {
	return ToolCallOptions{
		SchemaValidationEnabled:   true,
		RejectUnknownArguments:    false,
		PermissionMode:            ToolPermissionAllowAll,
		AllowedTools:              []string{},
		EmitProgressNotifications: true,
	}
}

func normalizePromptRenderOptions(options PromptRenderOptions) PromptRenderOptions {
	normalized := options
	normalized.Mode = strings.ToLower(strings.TrimSpace(normalized.Mode))
	if normalized.Mode == "" {
		normalized.Mode = PromptRenderingModeLegacy
	}
	if normalized.Mode != PromptRenderingModeStrict && normalized.Mode != PromptRenderingModeAdvanced {
		normalized.Mode = PromptRenderingModeLegacy
	}
	return normalized
}

func BuildToolsListResponse(msg jsonrpc.Request, tools []mcp.Tool) *jsonrpc.Response {
	sortedTools := append([]mcp.Tool(nil), tools...)
	sort.Slice(sortedTools, func(i, j int) bool {
		return sortedTools[i].Name < sortedTools[j].Name
	})

	start, err := ParseCursor(msg.Params, len(sortedTools))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	end := min(start+pageSize, len(sortedTools))

	result := map[string]any{
		"tools": sortedTools[start:end],
	}
	if end < len(sortedTools) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func BuildResourcesListResponse(msg jsonrpc.Request) *jsonrpc.Response {
	resources := defaultResources()
	start, err := ParseCursor(msg.Params, len(resources))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	end := min(start+pageSize, len(resources))

	result := map[string]any{
		"resources": resources[start:end],
	}
	if end < len(resources) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func BuildResourcesReadResponse(msg jsonrpc.Request, readResource func(string) (any, error)) *jsonrpc.Response {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid resources/read payload", nil)
	}
	if params.URI == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Resource URI is required", nil)
	}

	result, err := readResource(params.URI)
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInternalError), "Failed to encode resource result", nil)
	}

	return jsonrpc.NewResponse(msg.ID, map[string]any{
		"contents": []map[string]any{
			{
				"uri":      params.URI,
				"mimeType": "application/json",
				"text":     string(resultJSON),
			},
		},
	})
}

func BuildPromptsListResponse(msg jsonrpc.Request, catalog *promptcatalog.Registry) *jsonrpc.Response {
	if catalog == nil || !catalog.Enabled() {
		return semanticError(msg.ID, jsonrpc.ErrMethodNotFound, "Feature not supported", "not_supported", map[string]any{
			"feature": "prompt_catalog",
		})
	}

	if data, unavailable := promptCatalogUnavailableData(catalog); unavailable {
		return semanticError(msg.ID, jsonrpc.ErrServerError, "Resource temporarily unavailable", "not_available", data)
	}

	prompts := catalog.ListPrompts()
	start, err := ParseCursor(msg.Params, len(prompts))
	if err != nil {
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Invalid cursor value", "invalid_params", map[string]any{
			"field":   "cursor",
			"problem": "invalid_cursor",
		})
	}
	end := min(start+pageSize, len(prompts))

	list := make([]map[string]any, 0, end-start)
	for _, prompt := range prompts[start:end] {
		item := map[string]any{
			"name":        prompt.Name,
			"description": prompt.Description,
		}
		if prompt.Title != "" {
			item["title"] = prompt.Title
		}
		if len(prompt.Arguments) > 0 {
			args := make([]map[string]any, 0, len(prompt.Arguments))
			for _, argument := range prompt.Arguments {
				args = append(args, map[string]any{
					"name":     argument.Name,
					"required": argument.Required,
				})
			}
			item["arguments"] = args
		}
		list = append(list, item)
	}

	result := map[string]any{
		"prompts": list,
	}
	if end < len(prompts) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func BuildPromptsGetResponse(msg jsonrpc.Request, catalog *promptcatalog.Registry) *jsonrpc.Response {
	return BuildPromptsGetResponseWithOptions(msg, catalog, DefaultPromptRenderOptions())
}

func BuildPromptsGetResponseWithOptions(msg jsonrpc.Request, catalog *promptcatalog.Registry, options PromptRenderOptions) *jsonrpc.Response {
	if catalog == nil || !catalog.Enabled() {
		return semanticError(msg.ID, jsonrpc.ErrMethodNotFound, "Feature not supported", "not_supported", map[string]any{
			"feature": "prompt_catalog",
		})
	}

	if data, unavailable := promptCatalogUnavailableData(catalog); unavailable {
		return semanticError(msg.ID, jsonrpc.ErrServerError, "Resource temporarily unavailable", "not_available", data)
	}

	var params promptsGetParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return buildPromptsGetPayloadError(msg.ID, err)
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Prompt name is required", "invalid_params", map[string]any{
			"field":   "name",
			"problem": "missing",
		})
	}

	prompt, found := catalog.GetPrompt(params.Name)
	if !found {
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Unknown prompt name", "invalid_params", map[string]any{
			"field":   "name",
			"problem": "unknown_prompt",
			"value":   params.Name,
		})
	}

	normalizedOptions := normalizePromptRenderOptions(options)
	rawArgs := normalizePromptArgumentsRaw(params.Arguments)
	if strictErr := validateStrictPromptArguments(msg.ID, prompt.Template, prompt.Arguments, rawArgs, normalizedOptions); strictErr != nil {
		return strictErr
	}

	var renderedPrompt string
	var renderErr error
	switch normalizedOptions.Mode {
	case PromptRenderingModeAdvanced:
		governanceTier := governanceTierForPrompt(prompt, normalizedOptions)
		if governanceTier != "trusted" {
			return semanticError(msg.ID, jsonrpc.ErrMethodNotFound, "Advanced prompt rendering is blocked by governance policy", "not_supported", map[string]any{
				"feature": "prompt_catalog",
				"reason":  "governance_restricted",
			})
		}
		renderedPrompt, renderErr = renderPromptTemplateAdvanced(prompt.Template, rawArgs)
	default:
		normalizedArgs := normalizePromptArguments(params.Arguments)
		renderedPrompt, renderErr = renderPromptTemplate(prompt.Template, normalizedArgs)
	}
	if renderErr != nil {
		if !errors.Is(renderErr, errRenderedPromptTooLarge) {
			return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Prompt rendering failed", "invalid_params", map[string]any{
				"field":   "arguments",
				"problem": "render_error",
			})
		}
		return semanticError(msg.ID, jsonrpc.ErrInvalidParams, "Prompt arguments produced oversized output", "invalid_params", map[string]any{
			"field":    "arguments",
			"problem":  "rendered_prompt_too_large",
			"maxBytes": maxRenderedPromptBytes,
		})
	}
	return jsonrpc.NewResponse(msg.ID, map[string]any{
		"name":        prompt.Name,
		"description": prompt.Description,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": map[string]any{
					"type": "text",
					"text": renderedPrompt,
				},
			},
		},
	})
}

func buildPromptsGetPayloadError(id any, err error) *jsonrpc.Response {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := strings.TrimSpace(typeErr.Field)
		if strings.HasPrefix(field, "arguments") {
			return semanticError(id, jsonrpc.ErrInvalidParams, "Prompt arguments must be an object of string values", "invalid_params", map[string]any{
				"field":   "arguments",
				"problem": "invalid_type",
			})
		}
		if strings.HasPrefix(field, "name") {
			return semanticError(id, jsonrpc.ErrInvalidParams, "Prompt name must be a string", "invalid_params", map[string]any{
				"field":   "name",
				"problem": "invalid_type",
			})
		}
	}

	return semanticError(id, jsonrpc.ErrInvalidParams, "Invalid prompts/get payload", "invalid_params", map[string]any{
		"field":   "params",
		"problem": "malformed_payload",
	})
}

func BuildPingResponse(msg jsonrpc.Request) *jsonrpc.Response {
	return jsonrpc.NewResponse(msg.ID, map[string]any{})
}

func DispatchStandardMethodWithPromptOptions(msg jsonrpc.Request, toolManager *tools.Manager, catalog *promptcatalog.Registry, readResource func(string) (any, error), promptRenderOptions PromptRenderOptions) any {
	return DispatchStandardMethodWithOptions(msg, toolManager, catalog, readResource, promptRenderOptions, DefaultToolCallOptions())
}

func DispatchStandardMethodWithOptions(msg jsonrpc.Request, toolManager *tools.Manager, catalog *promptcatalog.Registry, readResource func(string) (any, error), promptRenderOptions PromptRenderOptions, toolCallOptions ToolCallOptions) any {
	switch msg.Method {
	case "tools/list":
		return BuildToolsListResponse(msg, toolManager.GetTools())
	case "resources/list":
		return BuildResourcesListResponse(msg)
	case "resources/read":
		return BuildResourcesReadResponse(msg, readResource)
	case "prompts/list":
		return BuildPromptsListResponse(msg, catalog)
	case "prompts/get":
		return BuildPromptsGetResponseWithOptions(msg, catalog, promptRenderOptions)
	case "tools/call":
		return BuildToolCallResponseWithContextAndOptions(msg, toolManager, readResource, ToolCallContext{}, toolCallOptions)
	case "ping":
		return BuildPingResponse(msg)
	default:
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", map[string]any{
				"method": msg.Method,
			})
		}
		return nil
	}
}

// DispatchStandardMethod handles shared non-initialize JSON-RPC methods for all transports.
func DispatchStandardMethod(msg jsonrpc.Request, toolManager *tools.Manager, catalog *promptcatalog.Registry, readResource func(string) (any, error)) any {
	return DispatchStandardMethodWithOptions(msg, toolManager, catalog, readResource, DefaultPromptRenderOptions(), DefaultToolCallOptions())
}

func semanticError(id any, code jsonrpc.ErrorCode, message, kind string, extra map[string]any) *jsonrpc.Response {
	data := map[string]any{
		"kind": kind,
	}
	maps.Copy(data, extra)
	return jsonrpc.NewErrorResponse(id, int(code), message, data)
}

func promptCatalogUnavailableData(catalog *promptcatalog.Registry) (map[string]any, bool) {
	if catalog == nil || catalog.PromptCount() > 0 {
		return nil, false
	}

	loadErrors := catalog.LoadErrors()
	if len(loadErrors) == 0 {
		return nil, false
	}

	return map[string]any{
		"feature":        "prompt_catalog",
		"loadErrorCount": len(loadErrors),
	}, true
}

func validateStrictPromptArguments(id any, template string, promptArguments []promptcatalog.PromptArgument, arguments map[string]string, options PromptRenderOptions) *jsonrpc.Response {
	if options.Mode != PromptRenderingModeStrict {
		return nil
	}

	requiredSet := make(map[string]struct{}, len(promptArguments))
	for _, promptArg := range promptArguments {
		key := strings.TrimSpace(promptArg.Name)
		if key == "" {
			continue
		}
		if _, exists := requiredSet[key]; exists {
			continue
		}
		requiredSet[key] = struct{}{}
	}

	for _, key := range extractTemplatePlaceholderKeys(template) {
		if _, exists := requiredSet[key]; exists {
			continue
		}
		requiredSet[key] = struct{}{}
	}

	requiredKeys := make([]string, 0, len(requiredSet))
	for key := range requiredSet {
		requiredKeys = append(requiredKeys, key)
	}
	sort.Strings(requiredKeys)

	missing := make([]string, 0)
	for _, key := range requiredKeys {
		if _, ok := arguments[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return semanticError(id, jsonrpc.ErrInvalidParams, "Missing required prompt arguments", "invalid_params", map[string]any{
			"field":   "arguments",
			"problem": "missing_required_arguments",
			"missing": missing,
		})
	}

	if !options.RejectUnknownArguments {
		return nil
	}

	unknown := make([]string, 0)
	for key := range arguments {
		if _, ok := requiredSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return semanticError(id, jsonrpc.ErrInvalidParams, "Unknown prompt arguments", "invalid_params", map[string]any{
			"field":   "arguments",
			"problem": "unknown_arguments",
			"unknown": unknown,
		})
	}
	return nil
}

func extractTemplatePlaceholderKeys(template string) []string {
	matches := promptcatalog.PromptPlaceholderPattern().FindAllStringSubmatch(template, -1)
	if len(matches) == 0 {
		return nil
	}
	keys := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		key := strings.TrimSpace(match[1])
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizePromptArguments(arguments map[string]string) map[string]string {
	normalizedRaw := normalizePromptArgumentsRaw(arguments)
	if len(normalizedRaw) == 0 {
		return nil
	}
	normalizedArgs := make(map[string]string, len(normalizedRaw))
	for key, value := range normalizedRaw {
		normalizedArgs[key] = normalizePromptArgumentValue(value)
	}
	return normalizedArgs
}

func normalizePromptArgumentsRaw(arguments map[string]string) map[string]string {
	rawKeys := make([]string, 0, len(arguments))
	for key := range arguments {
		rawKeys = append(rawKeys, key)
	}
	sort.Strings(rawKeys)

	normalizedArgs := make(map[string]string, len(rawKeys))
	for _, key := range rawKeys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		// Deterministic overwrite: later keys in sorted order win for the same trimmed key.
		normalizedArgs[trimmedKey] = strings.ReplaceAll(arguments[key], "\x00", "")
	}
	if len(normalizedArgs) == 0 {
		return nil
	}
	return normalizedArgs
}

func renderPromptTemplate(template string, normalizedArgs map[string]string) (string, error) {
	if template == "" || len(normalizedArgs) == 0 {
		return template, nil
	}

	matches := promptcatalog.PromptPlaceholderPattern().FindAllStringSubmatchIndex(template, -1)
	if len(matches) == 0 {
		return template, nil
	}

	var b strings.Builder
	b.Grow(len(template))
	last := 0

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start, end := match[0], match[1]
		keyStart, keyEnd := match[2], match[3]

		segment := template[last:start]
		if err := appendBounded(&b, segment); err != nil {
			return "", err
		}

		key := template[keyStart:keyEnd]
		if value, ok := normalizedArgs[key]; ok {
			seg := wrapPromptArgumentValue(key, value)
			if err := appendBounded(&b, seg); err != nil {
				return "", err
			}
		} else {
			// Strict mode: placeholder keys are matched exactly (case-sensitive).
			if err := appendBounded(&b, template[start:end]); err != nil {
				return "", err
			}
		}

		last = end
	}

	if err := appendBounded(&b, template[last:]); err != nil {
		return "", err
	}
	return b.String(), nil
}

func normalizePromptArgumentValue(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	raw, err := json.Marshal(value)
	if err != nil {
		return "\"\""
	}
	return string(raw)
}

func wrapPromptArgumentValue(key, value string) string {
	return fmt.Sprintf("<user_input name=%q format=\"json\">\n%s\n</user_input>", key, value)
}

func appendBounded(builder *strings.Builder, segment string) error {
	if builder.Len()+len(segment) > maxRenderedPromptBytes {
		return errRenderedPromptTooLarge
	}
	builder.WriteString(segment)
	return nil
}

func governanceTierForPrompt(prompt promptcatalog.Prompt, options PromptRenderOptions) string {
	if len(options.GovernanceRoots) == 0 {
		return "restricted"
	}
	sourcePath := canonicalPromptPath(prompt.SourcePath)
	bestPath := ""
	bestTier := "restricted"
	for _, root := range options.GovernanceRoots {
		rootPath := canonicalPromptPath(root.Path)
		if rootPath == "" {
			continue
		}
		rel, err := filepath.Rel(rootPath, sourcePath)
		if err != nil {
			continue
		}
		if rel != "." && rel != "" && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			continue
		}
		if len(rootPath) < len(bestPath) {
			continue
		}
		bestPath = rootPath
		tier := strings.ToLower(strings.TrimSpace(root.Tier))
		if tier == "" {
			tier = "restricted"
		}
		bestTier = tier
	}
	return bestTier
}

func canonicalPromptPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.Clean(trimmed)
	if abs, err := filepath.Abs(cleaned); err == nil {
		cleaned = abs
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	return filepath.Clean(cleaned)
}

func renderPromptTemplateAdvanced(templateSource string, args map[string]string) (string, error) {
	rewritten := rewriteAdvancedPromptTemplate(templateSource)
	tpl, err := template.New("prompt").
		Option("missingkey=zero").
		Funcs(advancedTemplateFuncMap()).
		Parse(rewritten)
	if err != nil {
		return "", err
	}

	data := map[string]any{
		"args": args,
	}
	for key, value := range args {
		data[key] = value
	}

	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", err
	}
	if b.Len() > maxRenderedPromptBytes {
		return "", errRenderedPromptTooLarge
	}
	return b.String(), nil
}

func rewriteAdvancedPromptTemplate(templateSource string) string {
	matches := promptcatalog.PromptPlaceholderPattern().FindAllStringSubmatchIndex(templateSource, -1)
	if len(matches) == 0 {
		return templateSource
	}

	var b strings.Builder
	b.Grow(len(templateSource))
	last := 0
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start, end := match[0], match[1]
		keyStart, keyEnd := match[2], match[3]
		token := strings.TrimSpace(templateSource[keyStart:keyEnd])

		b.WriteString(templateSource[last:start])
		switch {
		case token == "":
			b.WriteString(templateSource[start:end])
		case strings.HasPrefix(token, "."):
			b.WriteString(templateSource[start:end])
		default:
			if _, isControlToken := advancedTemplateControlTokens[token]; isControlToken {
				b.WriteString(templateSource[start:end])
			} else {
				b.WriteString("{{ index .args ")
				b.WriteString(strconv.Quote(token))
				b.WriteString(" }}")
			}
		}
		last = end
	}
	b.WriteString(templateSource[last:])
	return b.String()
}

func advancedTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"title":     strings.Title,
		"trim":      strings.TrimSpace,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace": func(input string, old string, newValue string) string {
			return strings.ReplaceAll(input, old, newValue)
		},
		"default": func(value string, fallback string) string {
			if strings.TrimSpace(value) == "" {
				return fallback
			}
			return value
		},
		"join": func(values []string, sep string) string {
			return strings.Join(values, sep)
		},
	}
}

func BuildToolCallResponse(msg jsonrpc.Request, toolManager *tools.Manager, readResource func(string) (any, error)) *jsonrpc.Response {
	return BuildToolCallResponseWithContextAndOptions(msg, toolManager, readResource, ToolCallContext{}, DefaultToolCallOptions())
}

func BuildToolCallResponseWithContext(msg jsonrpc.Request, toolManager *tools.Manager, readResource func(string) (any, error), callContext ToolCallContext) *jsonrpc.Response {
	return BuildToolCallResponseWithContextAndOptions(msg, toolManager, readResource, callContext, DefaultToolCallOptions())
}

func BuildToolCallResponseWithContextAndOptions(msg jsonrpc.Request, toolManager *tools.Manager, readResource func(string) (any, error), callContext ToolCallContext, options ToolCallOptions) *jsonrpc.Response {
	return toolpipeline.Execute(toolpipeline.ExecuteInput{
		Message:      msg,
		ToolManager:  toolManager,
		ReadResource: readResource,
		Context: toolpipeline.ToolCallContext{
			SessionID:               callContext.SessionID,
			RuntimeSessionID:        callContext.RuntimeSessionID,
			RuntimeCommandSessionID: callContext.RuntimeCommandSessionID,
			SessionInitialized:      callContext.SessionInitialized,
			MutatingAllowed:         callContext.MutatingAllowed,
		},
		Options: toolpipeline.ToolCallOptions{
			SchemaValidationEnabled:   options.SchemaValidationEnabled,
			RejectUnknownArguments:    options.RejectUnknownArguments,
			PermissionMode:            options.PermissionMode,
			AllowedTools:              options.AllowedTools,
			EmitProgressNotifications: options.EmitProgressNotifications,
		},
	})
}

func validateToolArguments(schema mcp.InputSchema, arguments map[string]any, rejectUnknown bool) *tooltypes.SemanticError {
	missingRequired := make([]string, 0)
	for _, required := range schema.Required {
		requiredKey := strings.TrimSpace(required)
		if requiredKey == "" {
			continue
		}
		if _, ok := arguments[requiredKey]; !ok {
			missingRequired = append(missingRequired, requiredKey)
		}
	}
	if len(missingRequired) > 0 {
		sort.Strings(missingRequired)
		return tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Missing required tool arguments", map[string]any{
			"field":   "arguments",
			"problem": "missing_required_arguments",
			"missing": missingRequired,
		})
	}

	if rejectUnknown {
		unknown := make([]string, 0)
		for argName := range arguments {
			if _, ok := schema.Properties[argName]; !ok {
				unknown = append(unknown, argName)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Unknown tool arguments", map[string]any{
				"field":   "arguments",
				"problem": "unknown_arguments",
				"unknown": unknown,
			})
		}
	}

	for argName, argValue := range arguments {
		propertySchemaRaw, exists := schema.Properties[argName]
		if !exists {
			continue
		}
		propertySchema, ok := propertySchemaRaw.(map[string]any)
		if !ok {
			continue
		}
		expectedType, _ := propertySchema["type"].(string)
		expectedType = strings.ToLower(strings.TrimSpace(expectedType))
		if expectedType == "" {
			continue
		}
		if !isJSONTypeMatch(argValue, expectedType) {
			return tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Tool argument has invalid type", map[string]any{
				"field":    argName,
				"problem":  "invalid_type",
				"expected": expectedType,
				"actual":   jsonTypeName(argValue),
			})
		}
	}

	return nil
}

func isJSONTypeMatch(value any, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		if !ok {
			return false
		}
		return number == float64(int64(number))
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "null":
		return value == nil
	default:
		return true
	}
}

func jsonTypeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "unknown"
	}
}

func emitToolCallLog(msg string, args ...any) {
	defer func() {
		_ = recover()
	}()
	logger.Info(msg, args...)
}

func BuildToolSuccessResult(toolName string, result any) map[string]any {
	return map[string]any{
		"type":              string(mcp.TypeResult),
		"tool":              toolName,
		"result":            result,
		"content":           ToolContentFromResult(result),
		"structuredContent": result,
		"isError":           false,
	}
}

func buildToolExecutionErrorResult(toolName string) map[string]any {
	return map[string]any{
		"type":    string(mcp.TypeResult),
		"tool":    toolName,
		"content": []map[string]any{{"type": "text", "text": toolExecutionErrorMessage}},
		"isError": true,
		"error": map[string]any{
			"kind": tooltypes.SemanticKindExecutionFailed,
		},
	}
}

func buildToolSemanticErrorResult(toolName string, semanticErr *tooltypes.SemanticError) map[string]any {
	message := "Tool is temporarily unavailable"
	if semanticErr != nil && strings.TrimSpace(semanticErr.Message) != "" {
		message = strings.TrimSpace(semanticErr.Message)
	}
	errorPayload := map[string]any{}
	if semanticErr != nil {
		if strings.TrimSpace(semanticErr.Kind) != "" {
			errorPayload["kind"] = strings.TrimSpace(semanticErr.Kind)
		}
		if semanticErr.Data != nil {
			maps.Copy(errorPayload, semanticErr.Data)
		}
	}
	return map[string]any{
		"type":    string(mcp.TypeResult),
		"tool":    toolName,
		"content": []map[string]any{{"type": "text", "text": message}},
		"isError": true,
		"error":   errorPayload,
	}
}

func enrichToolCallArguments(arguments map[string]any, callContext ToolCallContext, options ToolCallOptions, progressToken any, hasProgressToken bool) map[string]any {
	enriched := make(map[string]any, len(arguments)+1)
	maps.Copy(enriched, arguments)
	context := map[string]any{
		"session_id":                  strings.TrimSpace(callContext.SessionID),
		"runtime_session_id":          strings.TrimSpace(callContext.RuntimeSessionID),
		"runtime_command_session_id":  strings.TrimSpace(callContext.RuntimeCommandSessionID),
		"session_initialized":         callContext.SessionInitialized,
		"emit_progress_notifications": options.EmitProgressNotifications,
	}
	if hasProgressToken {
		context["progress_token"] = progressToken
	}
	enriched["_mcp"] = context
	return enriched
}

func extractProgressToken(meta map[string]any) (any, bool, error) {
	if len(meta) == 0 {
		return nil, false, nil
	}
	rawToken, exists := meta["progressToken"]
	if !exists {
		return nil, false, nil
	}
	switch token := rawToken.(type) {
	case string:
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, false, errors.New("Invalid progressToken in tools/call _meta")
		}
		return token, true, nil
	case float64:
		return token, true, nil
	default:
		return nil, false, errors.New("Invalid progressToken in tools/call _meta")
	}
}

func ToolContentFromResult(result any) []map[string]any {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return []map[string]any{{"type": "text", "text": "tool call completed"}}
	}
	return []map[string]any{{"type": "text", "text": string(resultJSON)}}
}

func ServerCapabilities(promptCatalogEnabled bool, promptListChanged bool) map[string]any {
	capabilities := map[string]any{
		"tools":     map[string]any{},
		"resources": map[string]any{},
	}
	if promptCatalogEnabled {
		capabilities["prompts"] = map[string]any{
			"listChanged": promptListChanged,
		}
	}
	return capabilities
}

func ParseCursor(paramsRaw json.RawMessage, total int) (int, error) {
	if len(paramsRaw) == 0 {
		return 0, nil
	}

	var params struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return 0, fmt.Errorf("invalid params payload")
	}
	if strings.TrimSpace(params.Cursor) == "" {
		return 0, nil
	}

	offset, err := strconv.Atoi(params.Cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor value")
	}
	if offset < 0 || offset > total {
		return 0, fmt.Errorf("invalid cursor value")
	}
	return offset, nil
}

func defaultResources() []map[string]any {
	return []map[string]any{
		{
			"uri":      "godot://project/info",
			"name":     "Project Info",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://scene/current",
			"name":     "Current Scene",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://script/current",
			"name":     "Current Script",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://policy/godot-checks",
			"name":     "Godot Policy Checks",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://runtime/metrics",
			"name":     "Runtime Metrics",
			"mimeType": "application/json",
		},
	}
}

// ParseJSONRPCFrame validates and parses one JSON-RPC message frame.
// Both stdio and streamable HTTP currently require a single message per frame.
func ParseJSONRPCFrame(frame []byte) ([]jsonrpc.Request, []any, bool, error) {
	trimmed := bytes.TrimSpace(frame)
	if len(trimmed) == 0 {
		return nil, nil, false, fmt.Errorf("empty message")
	}

	if trimmed[0] == '[' {
		return nil, []any{jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil)}, false, nil
	}

	rawMessages := []json.RawMessage{json.RawMessage(trimmed)}
	requests := make([]jsonrpc.Request, 0, len(rawMessages))
	prebuiltResponses := make([]any, 0)
	acceptedOneWay := false

	for _, rawMsg := range rawMessages {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(rawMsg, &envelope); err != nil {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
			continue
		}

		requestID, hasID, validID := parseIDFromEnvelope(envelope)
		if !validID {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		var msg jsonrpc.Request
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.Method == "" {
			_, hasResult := envelope["result"]
			_, hasErr := envelope["error"]
			if hasResult || hasErr {
				if msg.JSONRPC != jsonrpc.Version || !hasID || (hasResult && hasErr) {
					prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
				} else {
					acceptedOneWay = true
				}
				continue
			}
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.JSONRPC != jsonrpc.Version {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if rawParams, ok := envelope["params"]; ok && !isValidParamsValue(rawParams) {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.Method == "initialize" && msg.ID == nil {
			prebuiltResponses = append(prebuiltResponses, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		requests = append(requests, msg)
	}

	return requests, prebuiltResponses, acceptedOneWay, nil
}

func parseIDFromEnvelope(envelope map[string]json.RawMessage) (any, bool, bool) {
	rawID, exists := envelope["id"]
	if !exists {
		return nil, false, true
	}
	trimmed := bytes.TrimSpace(rawID)
	if len(trimmed) == 0 {
		return nil, true, false
	}

	var id any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&id); err != nil {
		return nil, true, false
	}
	if !isValidJSONRPCID(id) {
		return nil, true, false
	}
	return id, true, true
}

func isValidJSONRPCID(id any) bool {
	switch v := id.(type) {
	case string:
		return true
	case json.Number:
		return isJSONInteger(v.String())
	default:
		return false
	}
}

func isValidParamsValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{'
}

func isJSONInteger(value string) bool {
	if value == "" || strings.ContainsAny(value, ".eE") {
		return false
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return true
	}
	if strings.HasPrefix(value, "-") {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
