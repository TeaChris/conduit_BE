package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/conduit-platform/conduit/backend/internal/auth"
)

func newTestIssuer(t *testing.T) (*EdDSAIssuer, *SigningKey) {
	t.Helper()
	key, err := GenerateSigningKey("test-kid-1")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}
	issuer, err := NewEdDSAIssuer(key, DefaultIssuerConfig())
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}
	return issuer, key
}

func TestIssue_ValidToken(t *testing.T) {
	issuer, key := newTestIssuer(t)

	claims := auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	}

	tokenString, err := issuer.Issue(claims)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tokenString == "" {
		t.Fatal("Issue returned empty string")
	}

	// Parse and verify with the public key.
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}))
	if err != nil {
		t.Fatalf("jwt.Parse: %v", err)
	}
	if !parsed.Valid {
		t.Error("parsed token is not valid")
	}
}

func TestIssue_HeaderContents(t *testing.T) {
	issuer, _ := newTestIssuer(t)

	claims := auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	}

	tokenString, err := issuer.Issue(claims)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Parse without validation to inspect headers.
	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(tokenString, &conduitClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}

	// Check alg.
	if alg, ok := parsed.Header["alg"].(string); !ok || alg != "EdDSA" {
		t.Errorf("alg = %v, want EdDSA", parsed.Header["alg"])
	}

	// Check kid.
	if kid, ok := parsed.Header["kid"].(string); !ok || kid != "test-kid-1" {
		t.Errorf("kid = %v, want test-kid-1", parsed.Header["kid"])
	}

	// Check typ.
	if typ, ok := parsed.Header["typ"].(string); !ok || typ != "JWT" {
		t.Errorf("typ = %v, want JWT", parsed.Header["typ"])
	}
}

func TestIssue_RequiredClaims(t *testing.T) {
	issuer, _ := newTestIssuer(t)

	userID := uuid.New()
	sessionID := uuid.New()
	claims := auth.TokenClaims{
		UserID:    userID,
		TenantID:  uuid.New(),
		SessionID: sessionID,
	}

	tokenString, err := issuer.Issue(claims)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(tokenString, &conduitClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}

	cc, ok := parsed.Claims.(*conduitClaims)
	if !ok {
		t.Fatal("claims are not conduitClaims")
	}

	// iss
	if cc.Issuer != "conduit" {
		t.Errorf("iss = %q, want %q", cc.Issuer, "conduit")
	}

	// sub = UserID
	if cc.Subject != userID.String() {
		t.Errorf("sub = %q, want %q", cc.Subject, userID.String())
	}

	// aud
	if len(cc.Audience) != 1 || cc.Audience[0] != "conduit-api" {
		t.Errorf("aud = %v, want [conduit-api]", cc.Audience)
	}

	// jti — must be a valid UUID
	if _, err := uuid.Parse(cc.ID); err != nil {
		t.Errorf("jti is not a valid UUID: %q", cc.ID)
	}

	// sid = SessionID
	if cc.SessionID != sessionID.String() {
		t.Errorf("sid = %q, want %q", cc.SessionID, sessionID.String())
	}

	// iat and exp
	if cc.IssuedAt == nil {
		t.Error("iat is nil")
	}
	if cc.ExpiresAt == nil {
		t.Error("exp is nil")
	}
}

func TestIssue_CorrectExpiration(t *testing.T) {
	key, err := GenerateSigningKey("exp-test")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	cfg := IssuerConfig{
		Issuer:              "conduit",
		Audience:            "conduit-api",
		AccessTokenLifetime: 15 * time.Minute,
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

	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(tokenString, &conduitClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}

	cc := parsed.Claims.(*conduitClaims)
	diff := cc.ExpiresAt.Time.Sub(cc.IssuedAt.Time)

	// Allow a small tolerance for test execution time.
	if diff < 14*time.Minute || diff > 16*time.Minute {
		t.Errorf("exp - iat = %v, want ~15m", diff)
	}
}

func TestIssue_UniqueJTI(t *testing.T) {
	issuer, _ := newTestIssuer(t)

	claims := auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	}

	jtis := make(map[string]bool, 10)
	parser := jwt.NewParser()

	for i := 0; i < 10; i++ {
		tokenString, err := issuer.Issue(claims)
		if err != nil {
			t.Fatalf("Issue %d: %v", i, err)
		}

		parsed, _, err := parser.ParseUnverified(tokenString, &conduitClaims{})
		if err != nil {
			t.Fatalf("ParseUnverified %d: %v", i, err)
		}

		cc := parsed.Claims.(*conduitClaims)
		if jtis[cc.ID] {
			t.Fatalf("duplicate jti on iteration %d: %s", i, cc.ID)
		}
		jtis[cc.ID] = true
	}
}

func TestIssue_Ed25519SignatureVerifiable(t *testing.T) {
	issuer, key := newTestIssuer(t)

	tokenString, err := issuer.Issue(auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Verify signature with the correct public key.
	_, err = jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			t.Fatalf("unexpected signing method: %v", token.Header["alg"])
		}
		return key.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}

	// Verify with a different key should fail.
	wrongKey, err := GenerateSigningKey("wrong-key")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	_, err = jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return wrongKey.PublicKey, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}))
	if err == nil {
		t.Error("signature verification should fail with wrong key")
	}
}

func TestNewEdDSAIssuer_InvalidConfig(t *testing.T) {
	key, err := GenerateSigningKey("test-key")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	tests := []struct {
		name string
		key  *SigningKey
		cfg  IssuerConfig
	}{
		{"nil key", nil, DefaultIssuerConfig()},
		{"empty kid", &SigningKey{ID: "", PrivateKey: key.PrivateKey, PublicKey: key.PublicKey}, DefaultIssuerConfig()},
		{"nil private key", &SigningKey{ID: "test", PrivateKey: nil, PublicKey: key.PublicKey}, DefaultIssuerConfig()},
		{"empty issuer", key, IssuerConfig{Issuer: "", Audience: "aud", AccessTokenLifetime: time.Minute}},
		{"empty audience", key, IssuerConfig{Issuer: "iss", Audience: "", AccessTokenLifetime: time.Minute}},
		{"zero lifetime", key, IssuerConfig{Issuer: "iss", Audience: "aud", AccessTokenLifetime: 0}},
		{"negative lifetime", key, IssuerConfig{Issuer: "iss", Audience: "aud", AccessTokenLifetime: -time.Minute}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEdDSAIssuer(tt.key, tt.cfg)
			if err == nil {
				t.Error("expected error for invalid config, got nil")
			}
		})
	}
}

func TestIssue_NoPrivateKeyInError(t *testing.T) {
	// If signing fails (e.g., due to an invalid key), the error must not
	// contain private key material. The Issue method recovers panics from
	// the JWT library and returns a safe error.
	key, err := GenerateSigningKey("test-key")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}

	// Intentionally create an issuer with a truncated (invalid) private key
	// to trigger a panic inside the JWT library, which Issue() recovers.
	badKey := &SigningKey{
		ID:         "bad-key",
		PrivateKey: key.PrivateKey[:16], // Invalid key length
		PublicKey:  key.PublicKey,
	}

	// Bypass constructor validation to test the recovery path.
	issuer := &EdDSAIssuer{key: badKey, cfg: DefaultIssuerConfig()}

	_, err = issuer.Issue(auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error from signing with invalid key")
	}

	errMsg := err.Error()

	// The error must not contain key material or expose internals.
	if strings.Contains(errMsg, "slice bounds") {
		t.Error("error message leaks internal panic details")
	}

	// Should be our safe, generic error message.
	if !strings.Contains(errMsg, "invalid key configuration") {
		t.Errorf("unexpected error message: %s", errMsg)
	}
}

