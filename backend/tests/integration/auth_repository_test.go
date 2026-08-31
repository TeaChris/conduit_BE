//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/conduit-platform/conduit/backend/internal/auth"
	"github.com/conduit-platform/conduit/backend/internal/platform/database"
	"github.com/conduit-platform/conduit/backend/internal/platform/database/sqlcdb"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func getAuthTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://conduit:conduit@localhost:5432/conduit_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func createAuthTestTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	slug := fmt.Sprintf("auth-test-%s", uuid.New().String()[:8])
	var id pgtype.UUID
	err := pool.QueryRow(ctx,
		"INSERT INTO tenants (name, slug, status) VALUES ($1, $2, 'active') RETURNING id",
		"Auth Test Tenant", slug,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to create test tenant: %v", err)
	}
	uid, _ := authUUIDFromPgtype(id)
	return uid
}

func createAuthTestUser(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	q := sqlcdb.New(pool)
	email := fmt.Sprintf("auth-test-%s@example.com", uuid.New().String()[:8])
	user, err := q.CreateUser(ctx, sqlcdb.CreateUserParams{
		TenantID:    authUUIDToPgtype(tenantID),
		Email:       email,
		DisplayName: "Auth Test User",
		Status:      "active",
		Metadata:    []byte("{}"),
	})
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	uid, _ := authUUIDFromPgtype(user.ID)
	return uid
}

func authUUIDToPgtype(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}

func authUUIDFromPgtype(u pgtype.UUID) (uuid.UUID, error) {
	if !u.Valid {
		return uuid.Nil, fmt.Errorf("invalid pgtype.UUID")
	}
	return uuid.UUID(u.Bytes), nil
}

// ---------------------------------------------------------------------------
// Credential tests
// ---------------------------------------------------------------------------

func TestAuthCredential_CreateAndRetrieve(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	hash := "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$somehash"
	cred, err := repo.CreateCredential(ctx, tenantID, userID, hash)
	if err != nil {
		t.Fatalf("CreateCredential failed: %v", err)
	}
	if cred.UserID != userID {
		t.Errorf("UserID = %v, want %v", cred.UserID, userID)
	}
	if cred.PasswordHash != hash {
		t.Error("PasswordHash mismatch")
	}

	got, err := repo.GetCredentialByUserID(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("GetCredentialByUserID failed: %v", err)
	}
	if got.ID != cred.ID {
		t.Errorf("ID = %v, want %v", got.ID, cred.ID)
	}
}

func TestAuthCredential_UniquePerUser(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	_, err := repo.CreateCredential(ctx, tenantID, userID, "$argon2id$v=19$m=65536,t=3,p=4$salt$first")
	if err != nil {
		t.Fatalf("first CreateCredential failed: %v", err)
	}

	_, err = repo.CreateCredential(ctx, tenantID, userID, "$argon2id$v=19$m=65536,t=3,p=4$salt$second")
	if err == nil {
		t.Fatal("expected error for duplicate credential, got nil")
	}
}

func TestAuthCredential_UpdatePasswordHash(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	_, err := repo.CreateCredential(ctx, tenantID, userID, "$argon2id$v=19$m=65536,t=3,p=4$salt$old")
	if err != nil {
		t.Fatalf("CreateCredential failed: %v", err)
	}

	newHash := "$argon2id$v=19$m=65536,t=3,p=4$salt$new"
	updated, err := repo.UpdatePasswordHash(ctx, tenantID, userID, newHash)
	if err != nil {
		t.Fatalf("UpdatePasswordHash failed: %v", err)
	}
	if updated.PasswordHash != newHash {
		t.Errorf("PasswordHash = %v, want %v", updated.PasswordHash, newHash)
	}
}

func TestAuthCredential_NotFound(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	ctx := context.Background()

	_, err := repo.GetCredentialByUserID(ctx, tenantID, uuid.New())
	if err == nil {
		t.Fatal("expected error for nonexistent credential, got nil")
	}
}

func TestAuthCredential_InvalidUserFK(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	ctx := context.Background()

	_, err := repo.CreateCredential(ctx, tenantID, uuid.New(), "$argon2id$v=19$m=65536,t=3,p=4$salt$hash")
	if err == nil {
		t.Fatal("expected FK error for nonexistent user, got nil")
	}
}

// ---------------------------------------------------------------------------
// Session tests
// ---------------------------------------------------------------------------

func TestAuthSession_CreateAndRetrieve(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	ip := "192.168.1.1"
	ua := "TestAgent/1.0"
	session := &auth.Session{
		TenantID:         tenantID,
		UserID:           userID,
		FamilyID:         uuid.New(),
		RefreshTokenHash: "sha256_hash_of_refresh_token",
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
		IPAddress:        &ip,
		UserAgent:        &ua,
	}

	created, err := repo.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if created.UserID != userID {
		t.Errorf("UserID = %v, want %v", created.UserID, userID)
	}
	if created.IsRevoked() {
		t.Error("new session should not be revoked")
	}

	got, err := repo.GetSessionByID(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if got.FamilyID != created.FamilyID {
		t.Error("FamilyID mismatch")
	}
}

func TestAuthSession_RevokeSession(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	created, err := repo.CreateSession(ctx, &auth.Session{
		TenantID:         tenantID,
		UserID:           userID,
		FamilyID:         uuid.New(),
		RefreshTokenHash: "hash_to_revoke",
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = repo.RevokeSession(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("RevokeSession failed: %v", err)
	}

	got, err := repo.GetSessionByID(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("GetSessionByID after revoke failed: %v", err)
	}
	if !got.IsRevoked() {
		t.Error("session should be revoked")
	}
}

func TestAuthSession_RevokeAllUserSessions(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := repo.CreateSession(ctx, &auth.Session{
			TenantID:         tenantID,
			UserID:           userID,
			FamilyID:         uuid.New(),
			RefreshTokenHash: fmt.Sprintf("hash_%d", i),
			ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	err := repo.RevokeAllUserSessions(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("RevokeAllUserSessions failed: %v", err)
	}
}

func TestAuthSession_RefreshTokenRotation(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	familyID := uuid.New()
	created, err := repo.CreateSession(ctx, &auth.Session{
		TenantID:         tenantID,
		UserID:           userID,
		FamilyID:         familyID,
		RefreshTokenHash: "original_hash",
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	newHash := "rotated_hash"
	err = database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		txRepo := auth.NewPostgresRepositoryWithTx(tx)
		locked, err := txRepo.GetActiveSessionByFamilyIDForUpdate(ctx, familyID)
		if err != nil {
			return err
		}
		if locked.RefreshTokenHash != "original_hash" {
			return fmt.Errorf("unexpected hash: %s", locked.RefreshTokenHash)
		}
		_, err = txRepo.RotateRefreshToken(ctx, tenantID, locked.ID, newHash)
		return err
	})
	if err != nil {
		t.Fatalf("token rotation failed: %v", err)
	}

	got, err := repo.GetSessionByID(ctx, tenantID, created.ID)
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if got.RefreshTokenHash != newHash {
		t.Errorf("RefreshTokenHash = %v, want %v", got.RefreshTokenHash, newHash)
	}
}

func TestAuthSession_ConcurrentRefreshTokenReuse(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	familyID := uuid.New()
	originalHash := "concurrent_original_hash"
	_, err := repo.CreateSession(ctx, &auth.Session{
		TenantID:         tenantID,
		UserID:           userID,
		FamilyID:         familyID,
		RefreshTokenHash: originalHash,
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Two goroutines try to rotate the same token concurrently.
	// Only one should succeed; the other should see a mismatched hash (reuse detection).
	var wg sync.WaitGroup
	results := make(chan string, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(attempt int) {
			defer wg.Done()
			newHash := fmt.Sprintf("rotated_%d", attempt)
			txErr := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
				txRepo := auth.NewPostgresRepositoryWithTx(tx)
				locked, err := txRepo.GetActiveSessionByFamilyIDForUpdate(ctx, familyID)
				if err != nil {
					return err
				}
				if locked.RefreshTokenHash != originalHash {
					results <- "reuse_detected"
					return fmt.Errorf("reuse detected")
				}
				_, err = txRepo.RotateRefreshToken(ctx, tenantID, locked.ID, newHash)
				if err != nil {
					return err
				}
				results <- "success"
				return nil
			})
			if txErr != nil {
				select {
				case results <- "error":
				default:
				}
			}
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	reuseCount := 0
	for r := range results {
		switch r {
		case "success":
			successCount++
		case "reuse_detected":
			reuseCount++
		}
	}

	if successCount != 1 {
		t.Errorf("expected 1 success, got %d", successCount)
	}
	if reuseCount != 1 {
		t.Errorf("expected 1 reuse detection, got %d", reuseCount)
	}
}

func TestAuthSession_RevokeOtherSessions(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	var keepID uuid.UUID
	for i := 0; i < 3; i++ {
		s, err := repo.CreateSession(ctx, &auth.Session{
			TenantID:         tenantID,
			UserID:           userID,
			FamilyID:         uuid.New(),
			RefreshTokenHash: fmt.Sprintf("hash_%d", i),
			ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
		if i == 0 {
			keepID = s.ID
		}
	}

	err := repo.RevokeOtherUserSessions(ctx, tenantID, userID, keepID)
	if err != nil {
		t.Fatalf("RevokeOtherUserSessions failed: %v", err)
	}

	kept, err := repo.GetSessionByID(ctx, tenantID, keepID)
	if err != nil {
		t.Fatalf("GetSessionByID kept session failed: %v", err)
	}
	if kept.IsRevoked() {
		t.Error("kept session should not be revoked")
	}
}

func TestAuthSession_NotFound(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	ctx := context.Background()

	_, err := repo.GetSessionByID(ctx, tenantID, uuid.New())
	if err == nil {
		t.Fatal("expected error for nonexistent session, got nil")
	}
}

// ---------------------------------------------------------------------------
// Password Reset Token tests
// ---------------------------------------------------------------------------

func TestAuthPasswordResetToken_CreateAndRetrieve(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	tokenHash := "sha256_reset_" + uuid.New().String()[:8]
	created, err := repo.CreatePasswordResetToken(ctx, tenantID, userID, tokenHash, time.Now().Add(30*time.Minute))
	if err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}

	got, err := repo.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("GetPasswordResetTokenByHash failed: %v", err)
	}
	if got.ID != created.ID {
		t.Error("ID mismatch")
	}
}

func TestAuthPasswordResetToken_Consume(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	tokenHash := "sha256_consume_" + uuid.New().String()[:8]
	created, err := repo.CreatePasswordResetToken(ctx, tenantID, userID, tokenHash, time.Now().Add(30*time.Minute))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	err = repo.ConsumePasswordResetToken(ctx, created.ID)
	if err != nil {
		t.Fatalf("consume failed: %v", err)
	}

	_, err = repo.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err == nil {
		t.Fatal("expected error for consumed token")
	}
}

func TestAuthPasswordResetToken_DoubleConsumePrevented(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	tokenHash := "sha256_double_" + uuid.New().String()[:8]
	created, err := repo.CreatePasswordResetToken(ctx, tenantID, userID, tokenHash, time.Now().Add(30*time.Minute))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_ = repo.ConsumePasswordResetToken(ctx, created.ID)
	_ = repo.ConsumePasswordResetToken(ctx, created.ID) // no-op

	_, err = repo.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err == nil {
		t.Fatal("consumed token should not be retrievable")
	}
}

func TestAuthPasswordResetToken_InvalidateForUser(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	hash1 := "sha256_inv1_" + uuid.New().String()[:8]
	hash2 := "sha256_inv2_" + uuid.New().String()[:8]
	_, _ = repo.CreatePasswordResetToken(ctx, tenantID, userID, hash1, time.Now().Add(30*time.Minute))
	_, _ = repo.CreatePasswordResetToken(ctx, tenantID, userID, hash2, time.Now().Add(30*time.Minute))

	err := repo.InvalidatePasswordResetTokensForUser(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("invalidate failed: %v", err)
	}

	_, err = repo.GetPasswordResetTokenByHash(ctx, hash1)
	if err == nil {
		t.Error("first token should be invalidated")
	}
	_, err = repo.GetPasswordResetTokenByHash(ctx, hash2)
	if err == nil {
		t.Error("second token should be invalidated")
	}
}

// ---------------------------------------------------------------------------
// Email Verification Token tests
// ---------------------------------------------------------------------------

func TestAuthEmailVerificationToken_CreateAndRetrieve(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	tokenHash := "sha256_verify_" + uuid.New().String()[:8]
	created, err := repo.CreateEmailVerificationToken(ctx, tenantID, userID, tokenHash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	got, err := repo.GetEmailVerificationTokenByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.ID != created.ID {
		t.Error("ID mismatch")
	}
}

func TestAuthEmailVerificationToken_Consume(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	tokenHash := "sha256_vcons_" + uuid.New().String()[:8]
	created, err := repo.CreateEmailVerificationToken(ctx, tenantID, userID, tokenHash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	err = repo.ConsumeEmailVerificationToken(ctx, created.ID)
	if err != nil {
		t.Fatalf("consume failed: %v", err)
	}

	_, err = repo.GetEmailVerificationTokenByHash(ctx, tokenHash)
	if err == nil {
		t.Fatal("consumed token should not be retrievable")
	}
}

func TestAuthEmailVerificationToken_DoubleConsumePrevented(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	tokenHash := "sha256_vdbl_" + uuid.New().String()[:8]
	created, err := repo.CreateEmailVerificationToken(ctx, tenantID, userID, tokenHash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_ = repo.ConsumeEmailVerificationToken(ctx, created.ID)
	_ = repo.ConsumeEmailVerificationToken(ctx, created.ID) // no-op

	_, err = repo.GetEmailVerificationTokenByHash(ctx, tokenHash)
	if err == nil {
		t.Fatal("consumed token should not be retrievable")
	}
}

func TestAuthEmailVerificationToken_InvalidatePrevious(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	userID := createAuthTestUser(t, pool, tenantID)
	ctx := context.Background()

	hash1 := "sha256_vold_" + uuid.New().String()[:8]
	hash2 := "sha256_vnew_" + uuid.New().String()[:8]
	_, _ = repo.CreateEmailVerificationToken(ctx, tenantID, userID, hash1, time.Now().Add(24*time.Hour))

	_ = repo.InvalidateEmailVerificationTokensForUser(ctx, tenantID, userID)
	_, _ = repo.CreateEmailVerificationToken(ctx, tenantID, userID, hash2, time.Now().Add(24*time.Hour))

	_, err := repo.GetEmailVerificationTokenByHash(ctx, hash1)
	if err == nil {
		t.Error("old token should be invalidated")
	}
	_, err = repo.GetEmailVerificationTokenByHash(ctx, hash2)
	if err != nil {
		t.Fatalf("new token should be valid: %v", err)
	}
}

func TestAuthEmailVerificationToken_InvalidUserFK(t *testing.T) {
	pool := getAuthTestPool(t)
	repo := auth.NewPostgresRepository(pool)
	tenantID := createAuthTestTenant(t, pool)
	ctx := context.Background()

	_, err := repo.CreateEmailVerificationToken(ctx, tenantID, uuid.New(), "hash", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected FK error for nonexistent user")
	}
}
