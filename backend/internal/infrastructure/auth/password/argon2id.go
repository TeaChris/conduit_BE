package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/conduit-platform/conduit/backend/internal/auth"
)

// Compile-time interface check.
var _ auth.PasswordHasher = (*Argon2idHasher)(nil)

// Params holds Argon2id configuration. These parameters are centralized
// per TDR-0001 and security-standards §5.
type Params struct {
	Memory      uint32 // Memory in KiB (default: 65536 = 64 MiB).
	Iterations  uint32 // Time cost (default: 3).
	Parallelism uint8  // Thread count (default: 4).
	SaltLength  int    // Salt length in bytes (default: 16).
	KeyLength   uint32 // Derived key length in bytes (default: 32).
}

// DefaultParams returns the OWASP 2023 / TDR-0001 approved Argon2id parameters.
func DefaultParams() Params {
	return Params{
		Memory:      64 * 1024, // 64 MiB
		Iterations:  3,
		Parallelism: 4,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// Argon2idHasher implements auth.PasswordHasher using Argon2id.
type Argon2idHasher struct {
	params Params
}

// NewArgon2idHasher creates a new hasher with the given parameters.
// The parameters are validated on construction.
func NewArgon2idHasher(params Params) (*Argon2idHasher, error) {
	if params.Memory == 0 {
		return nil, errors.New("argon2id: memory must be > 0")
	}
	if params.Iterations == 0 {
		return nil, errors.New("argon2id: iterations must be > 0")
	}
	if params.Parallelism == 0 {
		return nil, errors.New("argon2id: parallelism must be > 0")
	}
	if params.SaltLength < 8 {
		return nil, errors.New("argon2id: salt length must be >= 8")
	}
	if params.KeyLength < 16 {
		return nil, errors.New("argon2id: key length must be >= 16")
	}
	return &Argon2idHasher{params: params}, nil
}

// Hash produces an Argon2id PHC-encoded hash string.
// A unique cryptographically random salt is generated for each call.
//
// Output format: $argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
func (h *Argon2idHasher) Hash(password string) (string, error) {
	// Generate a cryptographically secure random salt.
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2id: generating salt: %w", err)
	}

	// Derive the key. Password is not truncated.
	key := argon2.IDKey(
		[]byte(password),
		salt,
		h.params.Iterations,
		h.params.Memory,
		h.params.Parallelism,
		h.params.KeyLength,
	)

	// Encode as PHC string.
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedKey := base64.RawStdEncoding.EncodeToString(key)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.params.Memory,
		h.params.Iterations,
		h.params.Parallelism,
		encodedSalt,
		encodedKey,
	)

	return encoded, nil
}

// Verify checks a plaintext password against an Argon2id PHC-encoded hash.
// The parameters are parsed from the hash itself, not from the hasher's config.
// This ensures hashes created with older parameters still verify correctly.
func (h *Argon2idHasher) Verify(password, encodedHash string) (bool, error) {
	params, salt, storedKey, err := parsePHC(encodedHash)
	if err != nil {
		// Malformed hash — return false (not a match) plus the error
		// so the caller can distinguish "wrong password" from "corrupt data".
		// The service layer maps this appropriately without leaking details.
		return false, err
	}

	// Re-derive key using the parsed parameters and salt.
	computedKey := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	// Constant-time comparison to prevent timing attacks.
	if subtle.ConstantTimeCompare(storedKey, computedKey) == 1 {
		return true, nil
	}

	return false, nil
}

// parsePHC parses an Argon2id PHC-format encoded hash string.
// Expected format: $argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
func parsePHC(encoded string) (Params, []byte, []byte, error) {
	var params Params

	// Split on $ — expected: ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return params, nil, nil, errors.New("argon2id: invalid hash format")
	}

	if parts[1] != "argon2id" {
		return params, nil, nil, errors.New("argon2id: unsupported algorithm")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return params, nil, nil, errors.New("argon2id: invalid version")
	}
	if version != argon2.Version {
		return params, nil, nil, fmt.Errorf("argon2id: unsupported version %d", version)
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return params, nil, nil, errors.New("argon2id: invalid parameters")
	}
	params.Memory = memory
	params.Iterations = iterations
	params.Parallelism = parallelism

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return params, nil, nil, errors.New("argon2id: invalid salt encoding")
	}
	params.SaltLength = len(salt)

	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return params, nil, nil, errors.New("argon2id: invalid hash encoding")
	}
	params.KeyLength = uint32(len(key))

	return params, salt, key, nil
}
