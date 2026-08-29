package shared

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseJSONRPCFramePreservesRequestIDRepresentation(t *testing.T) {
	tests := []struct {
		name  string
		id    any
		frame string
	}{
		{
			name:  "string id",
			id:    "1",
			frame: `{"jsonrpc":"2.0","id":"1","method":"tools/list","params":{}}`,
		},
		{
			name:  "large integer id",
			id:    json.Number("9007199254740993"),
			frame: `{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/list","params":{}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests, responses, acceptedOneWay, err := ParseJSONRPCFrame([]byte(test.frame))
			if err != nil {
				t.Fatalf("parse frame: %v", err)
			}
			if len(responses) != 0 || acceptedOneWay {
				t.Fatalf("unexpected prebuilt response state: responses=%#v acceptedOneWay=%t", responses, acceptedOneWay)
			}
			if len(requests) != 1 {
				t.Fatalf("expected one request, got %d", len(requests))
			}
			if !reflect.DeepEqual(requests[0].ID, test.id) {
				t.Fatalf("expected request ID %#v (%T), got %#v (%T)", test.id, test.id, requests[0].ID, requests[0].ID)
			}
		})
	}
}
