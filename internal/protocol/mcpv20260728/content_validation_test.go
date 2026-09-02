package mcpv20260728

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/slighter12/godot-mcp-go/mcp"
)

func TestValidateCompleteResultAcceptsTypedEmptyEmbeddedText(t *testing.T) {
	result := mcp.CompleteResult{Content: []any{mcp.EmbeddedResourceContent{
		Type: "resource",
		Resource: mcp.ResourceContents{
			URI:  "file:///empty.txt",
			Text: "",
		},
	}}}

	if err := ValidateCompleteResult(result); err != nil {
		t.Fatalf("empty embedded text was rejected: %v", err)
	}
}

func TestIntegerValueUsesExactInt64Bounds(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
		ok    bool
	}{
		{name: "minimum", value: json.Number("-9223372036854775808"), want: math.MinInt64, ok: true},
		{name: "maximum", value: json.Number("9223372036854775807"), want: math.MaxInt64, ok: true},
		{name: "positive overflow", value: json.Number("9223372036854775808"), ok: false},
		{name: "negative overflow", value: json.Number("-9223372036854775809"), ok: false},
		{name: "large exact", value: json.Number("9007199254740993"), want: 9007199254740993, ok: true},
		{name: "integral exponent", value: json.Number("1e3"), want: 1000, ok: true},
		{name: "integral decimal", value: json.Number("1.0"), want: 1, ok: true},
		{name: "fractional", value: json.Number("1.5"), ok: false},
		{name: "float upper boundary", value: float64(1 << 63), ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := integerValue(test.value)
			if ok != test.ok || ok && got != test.want {
				t.Fatalf("integerValue(%v) = (%d, %v), want (%d, %v)", test.value, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestValidateElicitationSchemaComparesIntegerRangeExactly(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{
				"type":    "integer",
				"minimum": json.Number("9007199254740993"),
				"maximum": json.Number("9007199254740992"),
			},
		},
	}

	if err := validateElicitationSchema(schema); err == nil {
		t.Fatal("reversed large integer range was accepted")
	}
}

func TestValidateParameterHeaderSchemaRejectsAnnotationInNonObjectProperty(t *testing.T) {
	schema := mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"unsafe": []any{map[string]any{"x-mcp-header": "Injected"}},
		},
	}

	if err := ValidateParameterHeaderSchema(schema); err == nil {
		t.Fatal("annotation hidden in non-object property was accepted")
	}
}

func TestValidateCompleteResultRejectsTypedEmbeddedTextAndBlob(t *testing.T) {
	result := mcp.CompleteResult{Content: []any{mcp.EmbeddedResourceContent{
		Type: "resource",
		Resource: mcp.ResourceContents{
			URI:  "file:///ambiguous.txt",
			Text: "text",
			Blob: "YmxvYg==",
		},
	}}}

	if err := ValidateCompleteResult(result); err == nil {
		t.Fatal("embedded resource with text and blob was accepted")
	}
}
