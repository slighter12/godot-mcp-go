package mcpv20260728

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
)

const (
	requestStateTTL       = 5 * time.Minute
	requestStateClockSkew = 30 * time.Second
	maxRequestStateBytes  = 16 * 1024
	requestStateVersion   = "v2"
	legacyStateKeyID      = "legacy-default"
)

var ErrInvalidRequestState = errors.New("invalid requestState")
var ErrInvalidRoundTripInput = errors.New("invalid inputResponses")
var ErrInvalidRoundTripOutcome = errors.New("invalid multi round-trip result")
var ErrRequestStateEncoding = errors.New("failed to encode requestState")

type RequestStateBinding struct {
	Method      string
	Identity    string
	Parameters  any
	PrincipalID string
	// Deprecated: clientInfo is self-asserted and is not an authenticated
	// identity. It is retained only for source compatibility and is not bound.
	ClientInfo map[string]any
}

type RequestStateKey struct {
	ID  string
	Key []byte
}

type RequestStateKeyRing struct {
	Active      RequestStateKey
	DecryptOnly []RequestStateKey
}

type RequestStateCodec struct {
	activeID    string
	keys        map[string]cipher.AEAD
	now         func() time.Time
	nonceSource io.Reader
}

type requestStatePayload struct {
	Version            int                         `json:"v"`
	IssuedUnix         int64                       `json:"iat"`
	ExpiresUnix        int64                       `json:"exp"`
	Method             string                      `json:"method"`
	Identity           string                      `json:"identity"`
	ParamsDigest       string                      `json:"paramsDigest"`
	PrincipalDigest    string                      `json:"principalDigest,omitempty"`
	Round              int                         `json:"round"`
	Continuation       json.RawMessage             `json:"continuation,omitempty"`
	ExpectedRequests   map[string]mcp.InputRequest `json:"expectedRequests,omitempty"`
	CollectedResponses map[string]any              `json:"collectedResponses,omitempty"`
}

func NewRequestStateCodec(key []byte) (*RequestStateCodec, error) {
	return NewRequestStateCodecWithKeyRing(RequestStateKeyRing{Active: RequestStateKey{ID: legacyStateKeyID, Key: key}})
}

func NewRequestStateCodecWithKeyRing(ring RequestStateKeyRing) (*RequestStateCodec, error) {
	all := append([]RequestStateKey{ring.Active}, ring.DecryptOnly...)
	keys := make(map[string]cipher.AEAD, len(all))
	for _, entry := range all {
		if !validRequestStateKeyID(entry.ID) {
			return nil, errors.New("requestState key ID is invalid")
		}
		if len(entry.Key) < 32 {
			return nil, errors.New("requestState key must be at least 32 bytes")
		}
		if _, duplicate := keys[entry.ID]; duplicate {
			return nil, errors.New("requestState key IDs must be unique")
		}
		derived, err := hkdf.Key(sha256.New, entry.Key, nil, "godot-mcp requestState v2", 32)
		if err != nil {
			return nil, errors.New("derive requestState key")
		}
		block, err := aes.NewCipher(derived)
		if err != nil {
			return nil, errors.New("create requestState cipher")
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, errors.New("create requestState AEAD")
		}
		keys[entry.ID] = aead
	}
	if _, ok := keys[ring.Active.ID]; !ok {
		return nil, errors.New("requestState active key is required")
	}
	return &RequestStateCodec{activeID: ring.Active.ID, keys: keys, now: time.Now, nonceSource: rand.Reader}, nil
}

// ProcessRoundTrip validates and filters the current response round before a
// handler is entered, and protects all state needed for stateless re-entry.
func ProcessRoundTrip(ctx context.Context, codec *RequestStateCodec, binding RequestStateBinding, request mcp.RoundTripRequest, encodedState string, meta map[string]any, handler func(context.Context, mcp.RoundTripRequest) (mcp.RoundTripOutcome, error)) (any, error) {
	if codec == nil {
		return nil, ErrRequestStateEncoding
	}
	request.Round = 1
	if encodedState != "" {
		payload, err := codec.openPayload(encodedState, binding)
		if err != nil {
			return nil, ErrInvalidRequestState
		}
		request.Round, request.Continuation = payload.Round, payload.Continuation
		validated, missing, err := validateRoundTripResponses(request.InputResponses, request.InputResponsesPresent, payload.ExpectedRequests, payload.CollectedResponses)
		if err != nil {
			return nil, ErrInvalidRoundTripInput
		}
		if len(missing) != 0 {
			state, err := codec.sealPayload(binding, payload.Round, payload.Continuation, missing, validated)
			if err != nil {
				return nil, ErrRequestStateEncoding
			}
			return mcp.InputRequiredResult{ResultType: "input_required", Meta: meta, InputRequests: missing, RequestState: state}, nil
		}
		request.InputResponses = validated
	} else if request.InputResponsesPresent || request.InputResponses != nil {
		return nil, ErrInvalidRequestState
	}
	if ctx == nil {
		ctx = context.Background()
	}
	outcome, err := handler(ctx, request)
	if err != nil {
		return nil, err
	}
	if outcome.InputRequired != nil && outcome.Complete != nil || outcome.InputRequired == nil && outcome.Complete == nil {
		return nil, ErrInvalidRoundTripOutcome
	}
	if outcome.InputRequired == nil {
		return outcome.Complete, nil
	}
	spec := outcome.InputRequired
	if len(spec.InputRequests) == 0 && len(spec.Continuation) == 0 || len(spec.InputRequests) > 0 && validateInputRequests(spec.InputRequests) != nil {
		return nil, ErrInvalidRoundTripOutcome
	}
	state, err := codec.sealPayload(binding, request.Round+1, spec.Continuation, spec.InputRequests, nil)
	if err != nil {
		return nil, ErrRequestStateEncoding
	}
	return mcp.InputRequiredResult{ResultType: "input_required", Meta: meta, InputRequests: spec.InputRequests, RequestState: state}, nil
}

func (c *RequestStateCodec) Seal(binding RequestStateBinding, round int, continuation json.RawMessage) (string, error) {
	return c.sealPayload(binding, round, continuation, nil, nil)
}

func (c *RequestStateCodec) sealPayload(binding RequestStateBinding, round int, continuation json.RawMessage, expected map[string]mcp.InputRequest, collected map[string]any) (string, error) {
	if c == nil || round < 2 || len(continuation) != 0 && !json.Valid(continuation) {
		return "", ErrInvalidRequestState
	}
	now := c.now()
	payload := requestStatePayload{
		Version: 2, IssuedUnix: now.Unix(), ExpiresUnix: now.Add(requestStateTTL).Unix(), Method: binding.Method,
		Identity: binding.Identity, ParamsDigest: requestStateDigest(binding.Parameters), PrincipalDigest: requestStatePrincipalDigest(binding.PrincipalID),
		Round: round, Continuation: append(json.RawMessage(nil), continuation...), ExpectedRequests: expected, CollectedResponses: collected,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	aead := c.keys[c.activeID]
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(c.nonceSource, nonce); err != nil {
		return "", ErrRequestStateEncoding
	}
	keyID := base64.RawURLEncoding.EncodeToString([]byte(c.activeID))
	ciphertext := aead.Seal(nil, nonce, raw, requestStateAAD(c.activeID))
	encoded := requestStateVersion + "." + keyID + "." + base64.RawURLEncoding.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString(ciphertext)
	if len(encoded) > maxRequestStateBytes {
		return "", ErrInvalidRequestState
	}
	return encoded, nil
}

func (c *RequestStateCodec) Open(encoded string, binding RequestStateBinding) (int, json.RawMessage, error) {
	payload, err := c.openPayload(encoded, binding)
	if err != nil {
		return 0, nil, err
	}
	return payload.Round, append(json.RawMessage(nil), payload.Continuation...), nil
}

func (c *RequestStateCodec) openPayload(encoded string, binding RequestStateBinding) (requestStatePayload, error) {
	if c == nil || len(encoded) == 0 || len(encoded) > maxRequestStateBytes {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	parts := strings.Split(encoded, ".")
	if len(parts) != 4 || parts[0] != requestStateVersion {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	keyIDBytes, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	keyID := string(keyIDBytes)
	aead, ok := c.keys[keyID]
	if !ok || !validRequestStateKeyID(keyID) {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || len(nonce) != aead.NonceSize() {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	ciphertext, err := base64.RawURLEncoding.Strict().DecodeString(parts[3])
	if err != nil {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	raw, err := aead.Open(nil, nonce, ciphertext, requestStateAAD(keyID))
	if err != nil {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	var payload requestStatePayload
	now := c.now()
	if json.Unmarshal(raw, &payload) != nil || payload.Version != 2 || payload.Round < 2 ||
		now.After(time.Unix(payload.ExpiresUnix, 0).Add(requestStateClockSkew)) || time.Unix(payload.IssuedUnix, 0).After(now.Add(requestStateClockSkew)) ||
		payload.ExpiresUnix-payload.IssuedUnix != int64(requestStateTTL/time.Second) || payload.Method != binding.Method ||
		payload.Identity != binding.Identity || payload.ParamsDigest != requestStateDigest(binding.Parameters) ||
		payload.PrincipalDigest != requestStatePrincipalDigest(binding.PrincipalID) || len(payload.Continuation) != 0 && !json.Valid(payload.Continuation) {
		return requestStatePayload{}, ErrInvalidRequestState
	}
	return payload, nil
}

func validateInputRequests(requests map[string]mcp.InputRequest) error {
	for id, request := range requests {
		if strings.TrimSpace(id) == "" || request.Params == nil {
			return ErrInvalidRoundTripOutcome
		}
		switch request.Method {
		case "elicitation/create":
			if validateElicitationRequestParams(request.Params) != nil {
				return ErrInvalidRoundTripOutcome
			}
		case "sampling/createMessage", "roots/list":
		default:
			return ErrInvalidRoundTripOutcome
		}
	}
	return nil
}

func validateRoundTripResponses(responses map[string]any, present bool, expected map[string]mcp.InputRequest, collected map[string]any) (map[string]any, map[string]mcp.InputRequest, error) {
	if present && responses == nil {
		return nil, nil, ErrInvalidRoundTripInput
	}
	validated := make(map[string]any, len(collected)+len(responses))
	for id, value := range collected {
		validated[id] = value
	}
	for id, value := range responses {
		response, ok := value.(map[string]any)
		if !ok || !validAnyInputResponse(response) {
			return nil, nil, ErrInvalidRoundTripInput
		}
		request, known := expected[id]
		if !known {
			continue
		}
		if !validInputResponseForRequest(response, request) {
			return nil, nil, ErrInvalidRoundTripInput
		}
		validated[id] = response
	}
	missing := make(map[string]mcp.InputRequest)
	for id, request := range expected {
		if _, ok := validated[id]; !ok {
			missing[id] = request
		}
	}
	return validated, missing, nil
}

func validAnyInputResponse(response map[string]any) bool {
	return validElicitationResponse(response, nil) || validRootsResponse(response) || validSamplingResponse(response)
}

func validInputResponseForRequest(response map[string]any, request mcp.InputRequest) bool {
	switch request.Method {
	case "elicitation/create":
		return validElicitationResponse(response, request.Params)
	case "roots/list":
		return validRootsResponse(response)
	case "sampling/createMessage":
		return validSamplingResponse(response)
	default:
		return false
	}
}

func validElicitationResponse(response map[string]any, params map[string]any) bool {
	action, ok := response["action"].(string)
	if !ok || action != "accept" && action != "decline" && action != "cancel" {
		return false
	}
	if action != "accept" {
		return true
	}
	if params == nil {
		if content, present := response["content"]; present {
			_, ok := content.(map[string]any)
			return ok
		}
		return true
	}
	if params["mode"] == "url" {
		_, hasContent := response["content"]
		return !hasContent
	}
	content, ok := response["content"].(map[string]any)
	schema, schemaOK := params["requestedSchema"].(map[string]any)
	return ok && schemaOK && validateElicitationContent(content, schema)
}

func validRootsResponse(response map[string]any) bool {
	roots, ok := response["roots"].([]any)
	if !ok {
		return false
	}
	for _, value := range roots {
		root, ok := value.(map[string]any)
		if !ok {
			return false
		}
		uri, uriOK := root["uri"].(string)
		parsed, parseErr := url.Parse(uri)
		if !uriOK || parseErr != nil || !parsed.IsAbs() {
			return false
		}
		if name, present := root["name"]; present {
			if _, ok := name.(string); !ok {
				return false
			}
		}
	}
	return true
}

func validSamplingResponse(response map[string]any) bool {
	model, modelOK := response["model"].(string)
	role, roleOK := response["role"].(string)
	content, contentOK := response["content"].(map[string]any)
	if !modelOK || strings.TrimSpace(model) == "" || !roleOK || role != "user" && role != "assistant" || !contentOK {
		return false
	}
	typeName, _ := content["type"].(string)
	if typeName != "text" && typeName != "image" && typeName != "audio" || validateContentBlock(content) != nil {
		return false
	}
	if stopReason, present := response["stopReason"]; present {
		_, ok := stopReason.(string)
		return ok
	}
	return true
}

func stringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if item, ok := value.(string); ok {
				result = append(result, item)
			}
		}
		return result
	default:
		return nil
	}
}

func matchesRequestedType(value any, expected string) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if ok {
			_, err := number.Int64()
			return err == nil
		}
		float, ok := value.(float64)
		return ok && float == float64(int64(float))
	case "number":
		_, number := value.(json.Number)
		_, float := value.(float64)
		return number || float
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return false
	}
}

func containsJSONValue(values []any, wanted any) bool {
	wantedJSON, err := json.Marshal(wanted)
	if err != nil {
		return false
	}
	for _, value := range values {
		valueJSON, err := json.Marshal(value)
		if err == nil && string(valueJSON) == string(wantedJSON) {
			return true
		}
	}
	return false
}

func validRequestStateKeyID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._-", char) {
			continue
		}
		return false
	}
	return true
}

func requestStateAAD(keyID string) []byte {
	return []byte("godot-mcp requestState\x00" + requestStateVersion + "\x00" + ProtocolVersion + "\x00" + keyID)
}

func requestStatePrincipalDigest(principalID string) string {
	if principalID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(principalID))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func requestStateDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
