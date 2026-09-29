package jwt

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/conduit-platform/conduit/backend/internal/auth"
)

// testSetup creates a matched issuer + validator pair for testing.
func testSetup(t *testing.T) (*EdDSAIssuer, *EdDSAValidator, *SigningKey) {
	t.Helper()
	key, err := GenerateSigningKey("test-kid-v")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}
	resolver := NewStaticKeyResolver(*key)
	issuer, err := NewEdDSAIssuer(key, DefaultIssuerConfig())
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}
	return issuer, validator, key
}

func issueTestToken(t *testing.T, issuer *EdDSAIssuer) string {
	t.Helper()
	tokenString, err := issuer.Issue(auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tokenString
}

func TestValidate_ValidToken(t *testing.T) {
	issuer, validator, _ := testSetup(t)

	userID := uuid.New()
	sessionID := uuid.New()
	tokenString, err := issuer.Issue(auth.TokenClaims{
		UserID:    userID,
		TenantID:  uuid.New(),
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	validated, err := validator.Validate(tokenString)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if validated.UserID != userID {
		t.Errorf("UserID = %v, want %v", validated.UserID, userID)
	}
	if validated.SessionID != sessionID {
		t.Errorf("SessionID = %v, want %v", validated.SessionID, sessionID)
	}
	if validated.JTI == "" {
		t.Error("JTI is empty")
	}
	if validated.IssuedAt.IsZero() {
		t.Error("IssuedAt is zero")
	}
	if validated.ExpiresAt.IsZero() {
		t.Error("ExpiresAt is zero")
	}
}

func TestValidate_ExpiredToken(t *testing.T) {
	key, err := GenerateSigningKey("expired-test")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	// Issue with a very short lifetime.
	cfg := IssuerConfig{
		Issuer:              "conduit",
		Audience:            "conduit-api",
		AccessTokenLifetime: 1 * time.Millisecond,
	}
	issuer, err := NewEdDSAIssuer(key, cfg)
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}

	tokenString, err := issuer.Issue(auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Wait for expiration (plus clock skew buffer).
	time.Sleep(10 * time.Millisecond)

	resolver := NewStaticKeyResolver(*key)
	// Use zero clock skew to ensure expiration is detected.
	validator, err := NewEdDSAValidator(resolver, ValidatorConfig{
		Issuer:    "conduit",
		Audience:  "conduit-api",
		ClockSkew: 0,
	})
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if err != ErrTokenExpired {
		t.Errorf("error = %v, want ErrTokenExpired", err)
	}
}

func TestValidate_MalformedToken(t *testing.T) {
	_, validator, _ := testSetup(t)

	tests := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"random garbage", "not.a.jwt"},
		{"only dots", "..."},
		{"partial token", "eyJhbGciOiJFZERTQSJ9."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.Validate(tt.token)
			if err == nil {
				t.Error("expected error for malformed token")
			}
		})
	}
}

func TestValidate_InvalidSignature(t *testing.T) {
	issuer, _, _ := testSetup(t)
	tokenString := issueTestToken(t, issuer)

	// Tamper with the signature (last part of the JWT).
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatal("token does not have 3 parts")
	}
	// Flip a byte in the signature.
	sig := []byte(parts[2])
	if len(sig) > 0 {
		sig[0] = sig[0] ^ 0xFF
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)

	_, validator, _ := testSetup(t)
	_, err := validator.Validate(tampered)
	if err == nil {
		t.Error("expected error for tampered signature")
	}
}

func TestValidate_ModifiedPayload(t *testing.T) {
	issuer, _, key := testSetup(t)
	tokenString := issueTestToken(t, issuer)

	// Modify the payload while keeping the original signature.
	parts := strings.Split(tokenString, ".")
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	var claimsMap map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claimsMap); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	claimsMap["sub"] = uuid.New().String() // Change the user ID
	modified, err := json.Marshal(claimsMap)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	parts[1] = base64.RawURLEncoding.EncodeToString(modified)
	tampered := strings.Join(parts, ".")

	resolver := NewStaticKeyResolver(*key)
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	_, err = validator.Validate(tampered)
	if err == nil {
		t.Error("expected error for modified payload")
	}
}

func TestValidate_WrongSigningKey(t *testing.T) {
	// Issue with key A, validate with resolver containing only key B.
	keyA, err := GenerateSigningKey("key-a")
	if err != nil {
		t.Fatalf("GenerateSigningKey A: %v", err)
	}
	keyB, err := GenerateSigningKey("key-b")
	if err != nil {
		t.Fatalf("GenerateSigningKey B: %v", err)
	}

	issuer, err := NewEdDSAIssuer(keyA, DefaultIssuerConfig())
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}

	// Resolver only has key B — key A's kid won't resolve.
	resolver := NewStaticKeyResolver(*keyB)
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	tokenString := issueTestToken(t, issuer)
	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error when signing key is unknown to resolver")
	}
}

func TestValidate_UnknownKid(t *testing.T) {
	_, validator, _ := testSetup(t)

	// Create a token with a kid not in the resolver.
	unknownKey, err := GenerateSigningKey("unknown-kid")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}
	issuer, err := NewEdDSAIssuer(unknownKey, DefaultIssuerConfig())
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}

	tokenString := issueTestToken(t, issuer)
	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error for unknown kid")
	}
}

func TestValidate_WrongIssuer(t *testing.T) {
	key, err := GenerateSigningKey("wrong-iss")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	issuer, err := NewEdDSAIssuer(key, IssuerConfig{
		Issuer:              "wrong-issuer",
		Audience:            "conduit-api",
		AccessTokenLifetime: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}

	resolver := NewStaticKeyResolver(*key)
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	tokenString := issueTestToken(t, issuer)
	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error for wrong issuer")
	}
}

func TestValidate_WrongAudience(t *testing.T) {
	key, err := GenerateSigningKey("wrong-aud")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	issuer, err := NewEdDSAIssuer(key, IssuerConfig{
		Issuer:              "conduit",
		Audience:            "wrong-audience",
		AccessTokenLifetime: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}

	resolver := NewStaticKeyResolver(*key)
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	tokenString := issueTestToken(t, issuer)
	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error for wrong audience")
	}
}

func TestValidate_MissingSub(t *testing.T) {
	key, err := GenerateSigningKey("no-sub")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	// Manually create a token without the sub claim.
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, conduitClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "conduit",
			Subject:   "", // Missing sub
			Audience:  jwt.ClaimStrings{"conduit-api"},
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		SessionID: uuid.New().String(),
	})
	token.Header["kid"] = key.ID
	tokenString, err := token.SignedString(key.PrivateKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	resolver := NewStaticKeyResolver(*key)
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error for missing sub")
	}
}

func TestValidate_MissingSid(t *testing.T) {
	key, err := GenerateSigningKey("no-sid")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	// Token with valid sub but empty sid.
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, conduitClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "conduit",
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{"conduit-api"},
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		SessionID: "", // Missing sid
	})
	token.Header["kid"] = key.ID
	tokenString, err := token.SignedString(key.PrivateKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	resolver := NewStaticKeyResolver(*key)
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error for missing sid")
	}
}

func TestValidate_HS256Rejection(t *testing.T) {
	_, validator, key := testSetup(t)

	// Create an HS256 token using the public key as the HMAC secret
	// (classic algorithm confusion attack).
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, conduitClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "conduit",
			Subject:   uuid.New().String(),
			Audience:  jwt.ClaimStrings{"conduit-api"},
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		SessionID: uuid.New().String(),
	})
	token.Header["kid"] = key.ID
	tokenString, err := token.SignedString([]byte(key.PublicKey))
	if err != nil {
		t.Fatalf("SignedString HS256: %v", err)
	}

	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected rejection of HS256 token")
	}
}

func TestValidate_AlgNoneRejection(t *testing.T) {
	_, validator, key := testSetup(t)

	// Manually construct an alg=none token.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT","kid":"` + key.ID + `"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"conduit","sub":"` + uuid.New().String() + `","aud":["conduit-api"],"jti":"` + uuid.New().String() + `","sid":"` + uuid.New().String() + `","iat":` + "1700000000" + `,"exp":` + "9999999999" + `}`))
	tokenString := header + "." + payload + "."

	_, err := validator.Validate(tokenString)
	if err == nil {
		t.Error("expected rejection of alg=none token")
	}
}

func TestValidate_RS256Rejection(t *testing.T) {
	_, validator, _ := testSetup(t)

	// Construct a token claiming RS256 with garbage signature.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT","kid":"test-kid-v"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"conduit","sub":"` + uuid.New().String() + `","aud":["conduit-api"],"jti":"` + uuid.New().String() + `","sid":"` + uuid.New().String() + `","iat":1700000000,"exp":9999999999}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	tokenString := header + "." + payload + "." + sig

	_, err := validator.Validate(tokenString)
	if err == nil {
		t.Error("expected rejection of RS256 token")
	}
}

func TestValidate_IncorrectEdDSAKey(t *testing.T) {
	// Issue with one Ed25519 key, put a DIFFERENT key's public key under the same kid.
	keyA, err := GenerateSigningKey("shared-kid")
	if err != nil {
		t.Fatalf("GenerateSigningKey A: %v", err)
	}
	keyB, err := GenerateSigningKey("shared-kid") // Same kid, different key pair
	if err != nil {
		t.Fatalf("GenerateSigningKey B: %v", err)
	}

	issuer, err := NewEdDSAIssuer(keyA, DefaultIssuerConfig())
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}

	// Resolver has key B's public key under the same kid.
	resolver := NewStaticKeyResolver(*keyB)
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	tokenString := issueTestToken(t, issuer)
	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error when validating with wrong Ed25519 public key")
	}
}

func TestValidate_KeyRotationScenario(t *testing.T) {
	// Full key rotation lifecycle test.

	// 1. Generate key A and set up issuer + validator.
	keyA, err := GenerateSigningKey("key-A")
	if err != nil {
		t.Fatalf("GenerateSigningKey A: %v", err)
	}

	resolver := NewStaticKeyResolver(*keyA)
	issuerA, err := NewEdDSAIssuer(keyA, DefaultIssuerConfig())
	if err != nil {
		t.Fatalf("NewEdDSAIssuer A: %v", err)
	}
	validator, err := NewEdDSAValidator(resolver, DefaultValidatorConfig())
	if err != nil {
		t.Fatalf("NewEdDSAValidator: %v", err)
	}

	// 2. Issue token with key A.
	tokenA := issueTestToken(t, issuerA)

	// 3. Validate token A — should succeed.
	_, err = validator.Validate(tokenA)
	if err != nil {
		t.Fatalf("Validate token A (before rotation): %v", err)
	}

	// 4. Generate key B and introduce as the new active signing key.
	keyB, err := GenerateSigningKey("key-B")
	if err != nil {
		t.Fatalf("GenerateSigningKey B: %v", err)
	}
	resolver.AddKey("key-B", keyB.PublicKey)

	issuerB, err := NewEdDSAIssuer(keyB, DefaultIssuerConfig())
	if err != nil {
		t.Fatalf("NewEdDSAIssuer B: %v", err)
	}

	// 5. Issue token with key B.
	tokenB := issueTestToken(t, issuerB)

	// 6. Validate both tokens — both should succeed (A is still trusted).
	_, err = validator.Validate(tokenA)
	if err != nil {
		t.Fatalf("Validate token A (after B added): %v", err)
	}
	_, err = validator.Validate(tokenB)
	if err != nil {
		t.Fatalf("Validate token B: %v", err)
	}

	// 7. Retire key A.
	resolver.RemoveKey("key-A")

	// 8. Token A should now be rejected (unknown kid).
	_, err = validator.Validate(tokenA)
	if err == nil {
		t.Error("expected rejection of token A after key retirement")
	}

	// 9. Token B should still be valid.
	_, err = validator.Validate(tokenB)
	if err != nil {
		t.Fatalf("Validate token B (after A retired): %v", err)
	}
}

func TestNewEdDSAValidator_InvalidConfig(t *testing.T) {
	key, err := GenerateSigningKey("test")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}
	resolver := NewStaticKeyResolver(*key)

	tests := []struct {
		name     string
		resolver KeyResolver
		cfg      ValidatorConfig
	}{
		{"nil resolver", nil, DefaultValidatorConfig()},
		{"empty issuer", resolver, ValidatorConfig{Issuer: "", Audience: "aud", ClockSkew: time.Second}},
		{"empty audience", resolver, ValidatorConfig{Issuer: "iss", Audience: "", ClockSkew: time.Second}},
		{"negative clock skew", resolver, ValidatorConfig{Issuer: "iss", Audience: "aud", ClockSkew: -time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEdDSAValidator(tt.resolver, tt.cfg)
			if err == nil {
				t.Error("expected error for invalid config, got nil")
			}
		})
	}
}

