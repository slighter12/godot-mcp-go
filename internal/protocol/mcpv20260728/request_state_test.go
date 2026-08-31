package mcpv20260728

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRequestStateAEADKeyRingEncryptsAndRotates(t *testing.T) {
	oldKey := RequestStateKey{ID: "old", Key: []byte("0123456789abcdef0123456789abcdef")}
	newKey := RequestStateKey{ID: "new", Key: []byte("abcdef0123456789abcdef0123456789")}
	oldCodec, err := NewRequestStateCodecWithKeyRing(RequestStateKeyRing{Active: oldKey, DecryptOnly: []RequestStateKey{newKey}})
	if err != nil {
		t.Fatalf("create old codec: %v", err)
	}
	newCodec, err := NewRequestStateCodecWithKeyRing(RequestStateKeyRing{Active: newKey, DecryptOnly: []RequestStateKey{oldKey}})
	if err != nil {
		t.Fatalf("create new codec: %v", err)
	}
	binding := RequestStateBinding{Method: "tools/call", Identity: "godot.test", Parameters: map[string]any{"topic": "secret"}}
	token, err := oldCodec.Seal(binding, 2, json.RawMessage(`{"authorization":"server-private"}`))
	if err != nil {
		t.Fatalf("seal state: %v", err)
	}
	if !strings.HasPrefix(token, "v2.") || strings.Contains(token, "server-private") || strings.Contains(token, "authorization") {
		t.Fatalf("state token is not an opaque v2 envelope: %q", token)
	}
	round, continuation, err := newCodec.Open(token, binding)
	if err != nil || round != 2 || string(continuation) != `{"authorization":"server-private"}` {
		t.Fatalf("rotated codec could not open state: round=%d continuation=%q err=%v", round, continuation, err)
	}
}

func TestRequestStateCodecRejectsExpiredAndContextMismatchedState(t *testing.T) {
	codec, err := NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	issuedAt := time.Unix(1_800_000_000, 0)
	codec.now = func() time.Time { return issuedAt }
	binding := RequestStateBinding{Method: "tools/call", Identity: "godot.test.mrtr", Parameters: map[string]any{"topic": "MCP"}, PrincipalID: "subject-123"}
	state, err := codec.Seal(binding, 2, json.RawMessage(`{"step":1}`))
	if err != nil {
		t.Fatalf("seal state: %v", err)
	}

	changed := binding
	changed.Parameters = map[string]any{"topic": "different"}
	if _, _, err := codec.Open(state, changed); !errors.Is(err, ErrInvalidRequestState) {
		t.Fatalf("expected parameter-bound state rejection, got %v", err)
	}
	changed = binding
	changed.PrincipalID = ""
	if _, _, err := codec.Open(state, changed); !errors.Is(err, ErrInvalidRequestState) {
		t.Fatalf("expected principal-bound state rejection, got %v", err)
	}
	codec.now = func() time.Time { return issuedAt.Add(requestStateTTL + requestStateClockSkew + time.Second) }
	if _, _, err := codec.Open(state, binding); !errors.Is(err, ErrInvalidRequestState) {
		t.Fatalf("expected expired state rejection, got %v", err)
	}
}

func TestRequestStateCodecRequiresExplicitStrongKey(t *testing.T) {
	if _, err := NewRequestStateCodec(nil); err == nil {
		t.Fatal("expected missing production key to be rejected")
	}
	if _, err := NewRequestStateCodec([]byte("too-short")); err == nil {
		t.Fatal("expected short production key to be rejected")
	}
}

func TestRequestStateKeyRingRejectsUnsafeConfiguration(t *testing.T) {
	valid := []byte("0123456789abcdef0123456789abcdef")
	tests := []RequestStateKeyRing{
		{},
		{Active: RequestStateKey{ID: "active", Key: []byte("short")}},
		{Active: RequestStateKey{ID: "bad id", Key: valid}},
		{Active: RequestStateKey{ID: "same", Key: valid}, DecryptOnly: []RequestStateKey{{ID: "same", Key: valid}}},
	}
	for _, ring := range tests {
		if _, err := NewRequestStateCodecWithKeyRing(ring); err == nil {
			t.Fatalf("unsafe key ring was accepted: %#v", ring)
		}
	}
}

func TestRequestStateRejectsLegacyUnknownAndTamperedTokens(t *testing.T) {
	codec, err := NewRequestStateCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	binding := RequestStateBinding{Method: "tools/call", Identity: "godot.test"}
	token, err := codec.Seal(binding, 2, json.RawMessage(`{"step":1}`))
	if err != nil {
		t.Fatalf("seal state: %v", err)
	}
	parts := strings.Split(token, ".")
	ciphertext, err := base64.RawURLEncoding.Strict().DecodeString(parts[3])
	if err != nil || len(ciphertext) == 0 {
		t.Fatalf("decode ciphertext: %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 0x01
	parts[3] = base64.RawURLEncoding.EncodeToString(ciphertext)
	for _, invalid := range []string{"legacy.payload.signature", "v3.a.b.c", strings.Join(parts, ".")} {
		if _, _, err := codec.Open(invalid, binding); !errors.Is(err, ErrInvalidRequestState) {
			t.Fatalf("invalid token %q was accepted: %v", invalid, err)
		}
	}
}
