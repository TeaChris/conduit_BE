package password

import (
	"strings"
	"testing"
)

func TestHash_ValidPassword(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	encoded, err := h.Hash("securePassword123")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if encoded == "" {
		t.Fatal("Hash returned empty string")
	}

	// Verify PHC format structure.
	if !strings.HasPrefix(encoded, "$argon2id$v=19$") {
		t.Errorf("hash does not start with expected PHC prefix: %s", encoded)
	}
}

func TestHash_UniqueSalts(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	hash1, err := h.Hash("samePassword")
	if err != nil {
		t.Fatalf("Hash 1: %v", err)
	}

	hash2, err := h.Hash("samePassword")
	if err != nil {
		t.Fatalf("Hash 2: %v", err)
	}

	if hash1 == hash2 {
		t.Error("same password produced identical hashes — salt reuse detected")
	}
}

func TestVerify_CorrectPassword(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	password := "correctHorseBatteryStaple"
	encoded, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	match, err := h.Verify(password, encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify returned false for correct password")
	}
}

func TestVerify_IncorrectPassword(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	encoded, err := h.Hash("correctPassword")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	match, err := h.Verify("wrongPassword", encoded)
	if err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
	if match {
		t.Error("Verify returned true for incorrect password")
	}
}

func TestVerify_MalformedHash(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	tests := []struct {
		name string
		hash string
	}{
		{"empty string", ""},
		{"random garbage", "not-a-valid-hash"},
		{"wrong algorithm", "$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA"},
		{"missing parts", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA"},
		{"invalid version", "$argon2id$v=18$m=65536,t=3,p=4$c2FsdA$aGFzaA"},
		{"invalid params", "$argon2id$v=19$badparams$c2FsdA$aGFzaA"},
		{"invalid salt base64", "$argon2id$v=19$m=65536,t=3,p=4$!!!invalid!!!$aGFzaA"},
		{"invalid hash base64", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$!!!invalid!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := h.Verify("anyPassword", tt.hash)
			if err == nil {
				t.Error("Verify did not return error for malformed hash")
			}
			if match {
				t.Error("Verify returned true for malformed hash")
			}
		})
	}
}

func TestVerify_AlteredHash(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	password := "mySecurePassword"
	encoded, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	// Alter one character in the hash portion (last part after final $).
	parts := strings.Split(encoded, "$")
	hashPart := parts[len(parts)-1]
	if len(hashPart) > 0 {
		// Flip the first character.
		altered := make([]byte, len(hashPart))
		copy(altered, hashPart)
		if altered[0] == 'A' {
			altered[0] = 'B'
		} else {
			altered[0] = 'A'
		}
		parts[len(parts)-1] = string(altered)
	}
	alteredEncoded := strings.Join(parts, "$")

	match, err := h.Verify(password, alteredEncoded)
	if err != nil {
		// An error is acceptable if the altered base64 is invalid.
		return
	}
	if match {
		t.Error("Verify returned true for altered hash")
	}
}

func TestHash_ParametersEncoded(t *testing.T) {
	params := DefaultParams()
	h, err := NewArgon2idHasher(params)
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	encoded, err := h.Hash("testPassword")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	// Verify parameters are correctly encoded in the PHC string.
	expected := "$argon2id$v=19$m=65536,t=3,p=4$"
	if !strings.HasPrefix(encoded, expected) {
		t.Errorf("encoded hash does not contain expected parameters\ngot:  %s\nwant prefix: %s", encoded, expected)
	}
}

func TestHash_LongPassword(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	// 128-char password (maximum per security-standards §5).
	longPassword := strings.Repeat("a", 128)

	encoded, err := h.Hash(longPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	match, err := h.Verify(longPassword, encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify returned false for 128-char password")
	}
}

func TestHash_BoundaryMinPassword(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	// 12-char password (minimum per security-standards §5).
	password := "12characters"

	encoded, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	match, err := h.Verify(password, encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify returned false for 12-char password")
	}
}

func TestHash_EmptyPassword(t *testing.T) {
	h, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	// The hasher does not enforce password policy — that is the service layer's job.
	// Empty password should hash without error.
	encoded, err := h.Hash("")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	match, err := h.Verify("", encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify returned false for empty password")
	}
}

func TestHash_CustomParams(t *testing.T) {
	params := Params{
		Memory:      32 * 1024,
		Iterations:  2,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}

	h, err := NewArgon2idHasher(params)
	if err != nil {
		t.Fatalf("NewArgon2idHasher: %v", err)
	}

	password := "customParamsTest"
	encoded, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	expected := "$argon2id$v=19$m=32768,t=2,p=2$"
	if !strings.HasPrefix(encoded, expected) {
		t.Errorf("custom params not encoded correctly\ngot:  %s\nwant prefix: %s", encoded, expected)
	}

	match, err := h.Verify(password, encoded)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("Verify failed with custom params")
	}
}

func TestNewArgon2idHasher_InvalidParams(t *testing.T) {
	tests := []struct {
		name   string
		params Params
	}{
		{"zero memory", Params{Memory: 0, Iterations: 3, Parallelism: 4, SaltLength: 16, KeyLength: 32}},
		{"zero iterations", Params{Memory: 65536, Iterations: 0, Parallelism: 4, SaltLength: 16, KeyLength: 32}},
		{"zero parallelism", Params{Memory: 65536, Iterations: 3, Parallelism: 0, SaltLength: 16, KeyLength: 32}},
		{"salt too short", Params{Memory: 65536, Iterations: 3, Parallelism: 4, SaltLength: 4, KeyLength: 32}},
		{"key too short", Params{Memory: 65536, Iterations: 3, Parallelism: 4, SaltLength: 16, KeyLength: 8}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewArgon2idHasher(tt.params)
			if err == nil {
				t.Error("expected error for invalid params, got nil")
			}
		})
	}
}

func TestVerify_CrossParamsCompatibility(t *testing.T) {
	// Hash with one set of params, verify with a hasher that has different
	// default params. Verify must parse params from the hash itself.
	h1, err := NewArgon2idHasher(Params{
		Memory: 32 * 1024, Iterations: 2, Parallelism: 2,
		SaltLength: 16, KeyLength: 32,
	})
	if err != nil {
		t.Fatalf("NewArgon2idHasher h1: %v", err)
	}

	h2, err := NewArgon2idHasher(DefaultParams())
	if err != nil {
		t.Fatalf("NewArgon2idHasher h2: %v", err)
	}

	password := "crossParamsTest"
	encoded, err := h1.Hash(password)
	if err != nil {
		t.Fatalf("h1.Hash: %v", err)
	}

	// h2 has different default params but should verify using the
	// parameters encoded in the hash.
	match, err := h2.Verify(password, encoded)
	if err != nil {
		t.Fatalf("h2.Verify: %v", err)
	}
	if !match {
		t.Error("cross-params verification failed")
	}
}
