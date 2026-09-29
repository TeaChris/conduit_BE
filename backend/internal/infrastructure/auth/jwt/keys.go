package jwt

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
)

// SigningKey holds an Ed25519 key pair with its key ID (kid).
// The kid is included in JWT headers to support key rotation.
type SigningKey struct {
	ID         string             // Key identifier (kid) for JWT header.
	PrivateKey ed25519.PrivateKey // Used for signing. Never log this.
	PublicKey  ed25519.PublicKey  // Used for verification.
}

// ErrUnknownKeyID is returned when a kid cannot be resolved to a public key.
var ErrUnknownKeyID = errors.New("jwt: unknown key ID")

// KeyResolver resolves an Ed25519 public key by its key ID (kid).
// This abstraction supports key rotation: during rotation, multiple
// public keys are available for verification while only one private
// key is used for signing.
type KeyResolver interface {
	ResolveKey(kid string) (ed25519.PublicKey, error)
}

// StaticKeyResolver holds a set of Ed25519 public keys indexed by kid.
// It is safe for concurrent use.
//
// Key rotation workflow:
//  1. Add a new key pair (new kid becomes the active signing key).
//  2. Old public key remains in the resolver for verification.
//  3. After the access token lifetime (15 min) elapses, retire the old key.
type StaticKeyResolver struct {
	mu   sync.RWMutex
	keys map[string]ed25519.PublicKey
}

// NewStaticKeyResolver creates a key resolver pre-loaded with the given keys.
func NewStaticKeyResolver(keys ...SigningKey) *StaticKeyResolver {
	keyMap := make(map[string]ed25519.PublicKey, len(keys))
	for _, k := range keys {
		keyMap[k.ID] = k.PublicKey
	}
	return &StaticKeyResolver{keys: keyMap}
}

// ResolveKey returns the public key for the given kid, or ErrUnknownKeyID.
func (r *StaticKeyResolver) ResolveKey(kid string) (ed25519.PublicKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pk, ok := r.keys[kid]
	if !ok {
		return nil, ErrUnknownKeyID
	}
	return pk, nil
}

// AddKey adds a public key to the resolver. Used during key rotation
// to make a new key available for verification.
func (r *StaticKeyResolver) AddKey(kid string, publicKey ed25519.PublicKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys[kid] = publicKey
}

// RemoveKey removes a public key from the resolver. Used to retire
// an old key after all tokens signed with it have expired.
func (r *StaticKeyResolver) RemoveKey(kid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.keys, kid)
}

// GenerateSigningKey creates a new Ed25519 key pair with the given kid.
// Intended for testing and local development. Production keys should be
// generated offline and injected via environment variables.
func GenerateSigningKey(kid string) (*SigningKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("jwt: generating Ed25519 key: %w", err)
	}
	return &SigningKey{
		ID:         kid,
		PrivateKey: priv,
		PublicKey:  pub,
	}, nil
}

// ParseEd25519PrivateKeyPEM parses a PEM-encoded Ed25519 private key.
// This is used to load the signing key from the AUTH_JWT_PRIVATE_KEY
// environment variable. The PEM block type must be "PRIVATE KEY" (PKCS8).
//
// Never log the input or output of this function.
func ParseEd25519PrivateKeyPEM(pemData []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("jwt: failed to decode PEM block")
	}
	if block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("jwt: unexpected PEM block type %q, expected PRIVATE KEY", block.Type)
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("jwt: parsing PKCS8 private key: %w", err)
	}

	edKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("jwt: key is %T, not ed25519.PrivateKey", key)
	}

	return edKey, nil
}

// MarshalEd25519PrivateKeyPEM encodes an Ed25519 private key as PEM.
// Intended for test utilities only. Never use in production code paths.
func MarshalEd25519PrivateKeyPEM(key ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("jwt: marshaling PKCS8 private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}), nil
}
