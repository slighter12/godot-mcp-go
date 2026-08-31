package mcpv20260728

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const (
	maxInputResponsesBytes   = 256 * 1024
	maxInputResponsesEntries = 128
	maxInputResponsesDepth   = 32
	maxInputResponsesNodes   = 4096
)

var errInvalidInputResponsesPayload = errors.New("invalid inputResponses payload")

// DecodeInputResponses applies the released protocol's resource bounds before
// returning inputResponses to an MRTR handler.
func DecodeInputResponses(raw json.RawMessage) (map[string]any, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	if len(raw) > maxInputResponsesBytes {
		return nil, true, errInvalidInputResponsesPayload
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var responses map[string]any
	if err := decoder.Decode(&responses); err != nil || responses == nil {
		return nil, true, errInvalidInputResponsesPayload
	}
	if err := requireJSONEOF(decoder); err != nil || len(responses) > maxInputResponsesEntries {
		return nil, true, errInvalidInputResponsesPayload
	}
	nodes := 0
	if !boundedResponseValue(responses, 1, &nodes) {
		return nil, true, errInvalidInputResponsesPayload
	}
	return responses, true, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return errInvalidInputResponsesPayload
}

func boundedResponseValue(value any, depth int, nodes *int) bool {
	*nodes++
	if depth > maxInputResponsesDepth || *nodes > maxInputResponsesNodes {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if !boundedResponseValue(child, depth+1, nodes) {
				return false
			}
		}
	case []any:
		for _, child := range typed {
			if !boundedResponseValue(child, depth+1, nodes) {
				return false
			}
		}
	}
	return true
}
