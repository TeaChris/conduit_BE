package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/conduit-platform/conduit/backend/internal/auth"
)

// Compile-time interface check.
var _ auth.TokenIssuer = (*EdDSAIssuer)(nil)

// IssuerConfig holds configuration for JWT issuance.
type IssuerConfig struct {
	Issuer              string        // JWT "iss" claim (e.g., "conduit").
	Audience            string        // JWT "aud" claim (e.g., "conduit-api").
	AccessTokenLifetime time.Duration // Token validity duration. Default: 15 min.
}

// DefaultIssuerConfig returns production defaults per RFC-0002.
func DefaultIssuerConfig() IssuerConfig {
	return IssuerConfig{
		Issuer:              "conduit",
		Audience:            "conduit-api",
		AccessTokenLifetime: 15 * time.Minute,
	}
}

// EdDSAIssuer implements auth.TokenIssuer using Ed25519/EdDSA signing.
// The private key is held in memory and never logged or exposed through errors.
type EdDSAIssuer struct {
	key *SigningKey
	cfg IssuerConfig
}

// NewEdDSAIssuer creates a new JWT issuer with the given signing key and config.
func NewEdDSAIssuer(key *SigningKey, cfg IssuerConfig) (*EdDSAIssuer, error) {
	if key == nil {
		return nil, fmt.Errorf("jwt: signing key must not be nil")
	}
	if key.ID == "" {
		return nil, fmt.Errorf("jwt: signing key ID (kid) must not be empty")
	}
	if key.PrivateKey == nil {
		return nil, fmt.Errorf("jwt: private key must not be nil")
	}
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("jwt: issuer must not be empty")
	}
	if cfg.Audience == "" {
		return nil, fmt.Errorf("jwt: audience must not be empty")
	}
	if cfg.AccessTokenLifetime <= 0 {
		return nil, fmt.Errorf("jwt: access token lifetime must be > 0")
	}
	return &EdDSAIssuer{key: key, cfg: cfg}, nil
}

// conduitClaims extends jwt.RegisteredClaims with the session ID.
type conduitClaims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
}

// Issue creates a signed JWT access token with the required claims.
//
// JWT header: alg=EdDSA, kid=<key ID>, typ=JWT
// JWT claims: iss, sub, aud, jti, sid, iat, exp
//
// The jti is a unique UUID generated per token. Timestamps use UTC.
func (i *EdDSAIssuer) Issue(claims auth.TokenClaims) (signedToken string, issueErr error) {
	// Recover from panics in the JWT library (e.g., invalid key sizes).
	// The ed25519 package panics on malformed keys rather than returning
	// an error. We convert this to a safe error to avoid crashing the
	// application and to prevent key material from leaking through panics.
	defer func() {
		if r := recover(); r != nil {
			signedToken = ""
			issueErr = fmt.Errorf("jwt: signing failed due to invalid key configuration")
		}
	}()

	now := time.Now().UTC()
	jti := uuid.New().String()

	tokenClaims := conduitClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.cfg.Issuer,
			Subject:   claims.UserID.String(),
			Audience:  jwt.ClaimStrings{i.cfg.Audience},
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.cfg.AccessTokenLifetime)),
		},
		SessionID: claims.SessionID.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, tokenClaims)

	// Set the key ID and type in the JWT header.
	token.Header["kid"] = i.key.ID
	token.Header["typ"] = "JWT"

	// Sign with the Ed25519 private key. The library enforces that the
	// signing method is EdDSA and the key type is ed25519.PrivateKey.
	signed, err := token.SignedString(i.key.PrivateKey)
	if err != nil {
		// Do not include the private key or token content in the error.
		return "", fmt.Errorf("jwt: signing token: %w", err)
	}

	return signed, nil
}
