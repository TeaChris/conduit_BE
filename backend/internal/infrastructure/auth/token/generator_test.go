package token

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerate_NonEmpty(t *testing.T) {
	g, err := NewSecureGenerator(32)
	if err != nil {
		t.Fatalf("NewSecureGenerator: %v", err)
	}

	raw, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if raw == "" {
		t.Fatal("Generate returned empty string")
	}
}

func TestGenerate_Uniqueness(t *testing.T) {
	g, err := NewSecureGenerator(32)
	if err != nil {
		t.Fatalf("NewSecureGenerator: %v", err)
	}

	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		raw, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate %d: %v", i, err)
		}
		if seen[raw] {
			t.Fatalf("duplicate token on iteration %d", i)
		}
		seen[raw] = true
	}
}

func TestGenerate_ValidBase64URL(t *testing.T) {
	g, err := NewSecureGenerator(32)
	if err != nil {
		t.Fatalf("NewSecureGenerator: %v", err)
	}

	raw, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Must decode as valid base64url without padding.
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("token is not valid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("decoded length = %d, want 32", len(decoded))
	}
}

func TestGenerate_NoBase64Padding(t *testing.T) {
	g, err := NewSecureGenerator(32)
	if err != nil {
		t.Fatalf("NewSecureGenerator: %v", err)
	}

	raw, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if strings.Contains(raw, "=") {
		t.Error("token contains base64 padding character '='")
	}
}

func TestHash_Deterministic(t *testing.T) {
	g, err := NewSecureGenerator(32)
	if err != nil {
		t.Fatalf("NewSecureGenerator: %v", err)
	}

	token := "test-token-value"
	hash1 := g.Hash(token)
	hash2 := g.Hash(token)

	if hash1 != hash2 {
		t.Errorf("Hash is not deterministic: %q != %q", hash1, hash2)
	}
}

func TestHash_DifferentInputsDifferentOutputs(t *testing.T) {
	g, err := NewSecureGenerator(32)
	if err != nil {
		t.Fatalf("NewSecureGenerator: %v", err)
	}

	hash1 := g.Hash("token-one")
	hash2 := g.Hash("token-two")

	if hash1 == hash2 {
		t.Error("different inputs produced same hash")
	}
}

func TestHash_OutputFormat(t *testing.T) {
	g, err := NewSecureGenerator(32)
	if err != nil {
		t.Fatalf("NewSecureGenerator: %v", err)
	}

	hash := g.Hash("test-token")

	// SHA-256 hex digest should be 64 characters.
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}

	// Must be valid hex.
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("hash contains non-hex character: %c", c)
			break
		}
	}
}

func TestNewSecureGenerator_InvalidByteLength(t *testing.T) {
	tests := []struct {
		name       string
		byteLength int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too short", 15},
		{"minimum minus one", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSecureGenerator(tt.byteLength)
			if err == nil {
				t.Error("expected error for invalid byte length, got nil")
			}
		})
	}
}

func TestNewSecureGenerator_ValidMinimum(t *testing.T) {
	g, err := NewSecureGenerator(16) // Minimum valid
	if err != nil {
		t.Fatalf("NewSecureGenerator(16): %v", err)
	}

	raw, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("not valid base64url: %v", err)
	}
	if len(decoded) != 16 {
		t.Errorf("decoded length = %d, want 16", len(decoded))
	}
}
