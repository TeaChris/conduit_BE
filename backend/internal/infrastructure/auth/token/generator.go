package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/conduit-platform/conduit/backend/internal/auth"
)

// Compile-time interface check.
var _ auth.TokenGenerator = (*SecureGenerator)(nil)

// SecureGenerator implements auth.TokenGenerator using crypto/rand for
// random token generation and SHA-256 for deterministic token hashing.
//
// Security rationale:
// - crypto/rand provides cryptographically secure randomness (CSPRNG).
// - SHA-256 provides a one-way hash for opaque tokens before persistence.
//   Opaque tokens are random with high entropy, so SHA-256 is appropriate
//   (unlike passwords which require memory-hard hashing). Per RFC-0002 §13.
type SecureGenerator struct {
	byteLength int
}

// NewSecureGenerator creates a new token generator.
// byteLength controls the number of random bytes generated per token.
// Minimum 16 bytes (128 bits); default recommendation is 32 (256 bits).
func NewSecureGenerator(byteLength int) (*SecureGenerator, error) {
	if byteLength < 16 {
		return nil, errors.New("token: byte length must be >= 16 for sufficient entropy")
	}
	return &SecureGenerator{byteLength: byteLength}, nil
}

// Generate creates a cryptographically random token and returns it as a
// base64url-encoded string (no padding). Each call produces a unique token.
//
// No math/rand. No timestamps. No UUID-only construction.
func (g *SecureGenerator) Generate() (string, error) {
	buf := make([]byte, g.byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("token: reading crypto/rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Hash returns the SHA-256 hex digest of the given token string.
// This is deterministic: same input always produces the same output.
// The hex representation is suitable for PostgreSQL text column storage.
//
// SHA-256 is appropriate for hashing high-entropy opaque tokens.
// Passwords require memory-hard hashing (Argon2id) instead.
func (g *SecureGenerator) Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
