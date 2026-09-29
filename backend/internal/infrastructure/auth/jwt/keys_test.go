package jwt

import (
	"crypto/ed25519"
	"testing"
)

func TestGenerateSigningKey(t *testing.T) {
	key, err := GenerateSigningKey("test-key-1")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	if key.ID != "test-key-1" {
		t.Errorf("key ID = %q, want %q", key.ID, "test-key-1")
	}
	if key.PrivateKey == nil {
		t.Fatal("private key is nil")
	}
	if key.PublicKey == nil {
		t.Fatal("public key is nil")
	}
	if len(key.PrivateKey) != ed25519.PrivateKeySize {
		t.Errorf("private key length = %d, want %d", len(key.PrivateKey), ed25519.PrivateKeySize)
	}
	if len(key.PublicKey) != ed25519.PublicKeySize {
		t.Errorf("public key length = %d, want %d", len(key.PublicKey), ed25519.PublicKeySize)
	}
}

func TestStaticKeyResolver_ResolveKnownKey(t *testing.T) {
	key, err := GenerateSigningKey("key-1")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	resolver := NewStaticKeyResolver(*key)

	pub, err := resolver.ResolveKey("key-1")
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if !pub.Equal(key.PublicKey) {
		t.Error("resolved key does not match expected public key")
	}
}

func TestStaticKeyResolver_UnknownKey(t *testing.T) {
	resolver := NewStaticKeyResolver()

	_, err := resolver.ResolveKey("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if err != ErrUnknownKeyID {
		t.Errorf("error = %v, want ErrUnknownKeyID", err)
	}
}

func TestStaticKeyResolver_MultipleKeys(t *testing.T) {
	key1, err := GenerateSigningKey("key-1")
	if err != nil {
		t.Fatalf("GenerateSigningKey 1: %v", err)
	}
	key2, err := GenerateSigningKey("key-2")
	if err != nil {
		t.Fatalf("GenerateSigningKey 2: %v", err)
	}

	resolver := NewStaticKeyResolver(*key1, *key2)

	pub1, err := resolver.ResolveKey("key-1")
	if err != nil {
		t.Fatalf("ResolveKey key-1: %v", err)
	}
	if !pub1.Equal(key1.PublicKey) {
		t.Error("key-1 mismatch")
	}

	pub2, err := resolver.ResolveKey("key-2")
	if err != nil {
		t.Fatalf("ResolveKey key-2: %v", err)
	}
	if !pub2.Equal(key2.PublicKey) {
		t.Error("key-2 mismatch")
	}
}

func TestStaticKeyResolver_AddKey(t *testing.T) {
	resolver := NewStaticKeyResolver()

	key, err := GenerateSigningKey("added-key")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	// Should not resolve before adding.
	_, err = resolver.ResolveKey("added-key")
	if err == nil {
		t.Fatal("key should not be resolvable before AddKey")
	}

	resolver.AddKey("added-key", key.PublicKey)

	// Should resolve after adding.
	pub, err := resolver.ResolveKey("added-key")
	if err != nil {
		t.Fatalf("ResolveKey after AddKey: %v", err)
	}
	if !pub.Equal(key.PublicKey) {
		t.Error("added key mismatch")
	}
}

func TestStaticKeyResolver_RemoveKey(t *testing.T) {
	key, err := GenerateSigningKey("removable-key")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	resolver := NewStaticKeyResolver(*key)

	// Should resolve before removal.
	_, err = resolver.ResolveKey("removable-key")
	if err != nil {
		t.Fatalf("ResolveKey before remove: %v", err)
	}

	resolver.RemoveKey("removable-key")

	// Should not resolve after removal.
	_, err = resolver.ResolveKey("removable-key")
	if err == nil {
		t.Fatal("key should not be resolvable after RemoveKey")
	}
}

func TestParseMarshalEd25519PrivateKeyPEM(t *testing.T) {
	key, err := GenerateSigningKey("pem-test")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	// Marshal to PEM.
	pemData, err := MarshalEd25519PrivateKeyPEM(key.PrivateKey)
	if err != nil {
		t.Fatalf("MarshalEd25519PrivateKeyPEM: %v", err)
	}
	if len(pemData) == 0 {
		t.Fatal("PEM data is empty")
	}

	// Parse back from PEM.
	parsed, err := ParseEd25519PrivateKeyPEM(pemData)
	if err != nil {
		t.Fatalf("ParseEd25519PrivateKeyPEM: %v", err)
	}

	if !parsed.Equal(key.PrivateKey) {
		t.Error("parsed private key does not match original")
	}
}

func TestParseEd25519PrivateKeyPEM_InvalidPEM(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"garbage", []byte("not a pem block")},
		{"wrong type", []byte("-----BEGIN RSA PRIVATE KEY-----\naGVsbG8=\n-----END RSA PRIVATE KEY-----")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEd25519PrivateKeyPEM(tt.data)
			if err == nil {
				t.Error("expected error for invalid PEM, got nil")
			}
		})
	}
}
