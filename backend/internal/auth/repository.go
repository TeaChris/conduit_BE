package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/conduit-platform/conduit/backend/internal/platform/database"
	"github.com/conduit-platform/conduit/backend/internal/platform/database/sqlcdb"
	platformerrors "github.com/conduit-platform/conduit/backend/internal/platform/errors"
)

// Repository defines the persistence contract for the Authentication domain.
// The application/service layer depends on this interface, not on PostgreSQL or sqlc.
type Repository interface {
	// --- Credentials ---
	CreateCredential(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error)
	GetCredentialByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*Credential, error)
	UpdatePasswordHash(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error)

	// --- Sessions ---
	CreateSession(ctx context.Context, session *Session) (*Session, error)
	GetSessionByID(ctx context.Context, tenantID, sessionID uuid.UUID) (*Session, error)
	// GetActiveSessionByFamilyIDForUpdate retrieves the active session for a token family
	// with a FOR UPDATE lock. This is the critical operation for refresh-token concurrency.
	// The caller MUST be within a transaction (use WithTx).
	GetActiveSessionByFamilyIDForUpdate(ctx context.Context, familyID uuid.UUID) (*Session, error)
	RotateRefreshToken(ctx context.Context, tenantID, sessionID uuid.UUID, newTokenHash string) (*Session, error)
	RevokeSession(ctx context.Context, tenantID, sessionID uuid.UUID) error
	RevokeAllUserSessions(ctx context.Context, tenantID, userID uuid.UUID) error
	RevokeOtherUserSessions(ctx context.Context, tenantID, userID, currentSessionID uuid.UUID) error

	// --- Password Reset Tokens ---
	CreatePasswordResetToken(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*PasswordResetToken, error)
	GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	ConsumePasswordResetToken(ctx context.Context, tokenID uuid.UUID) error
	InvalidatePasswordResetTokensForUser(ctx context.Context, tenantID, userID uuid.UUID) error

	// --- Email Verification Tokens ---
	CreateEmailVerificationToken(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*EmailVerificationToken, error)
	GetEmailVerificationTokenByHash(ctx context.Context, tokenHash string) (*EmailVerificationToken, error)
	ConsumeEmailVerificationToken(ctx context.Context, tokenID uuid.UUID) error
	InvalidateEmailVerificationTokensForUser(ctx context.Context, tenantID, userID uuid.UUID) error
}

// PostgresRepository implements Repository using PostgreSQL via sqlc-generated queries.
type PostgresRepository struct {
	queries *sqlcdb.Queries
}

// NewPostgresRepository creates a new PostgresRepository.
// The pool satisfies sqlcdb.DBTX, so sqlc uses it directly.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{
		queries: sqlcdb.New(pool),
	}
}

// NewPostgresRepositoryWithTx creates a PostgresRepository bound to a transaction.
// Used when multiple repository operations must execute atomically (e.g., registration).
func NewPostgresRepositoryWithTx(tx pgx.Tx) *PostgresRepository {
	return &PostgresRepository{
		queries: sqlcdb.New(tx),
	}
}

// ---------------------------------------------------------------------------
// Credentials
// ---------------------------------------------------------------------------

func (r *PostgresRepository) CreateCredential(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error) {
	result, err := r.queries.CreateCredential(ctx, sqlcdb.CreateCredentialParams{
		TenantID:     uuidToPgtype(tenantID),
		UserID:       uuidToPgtype(userID),
		PasswordHash: passwordHash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCredentialAlreadyExists
		}
		return nil, platformerrors.NewInfraError("auth.repository.CreateCredential", err)
	}
	return toCredential(result)
}

func (r *PostgresRepository) GetCredentialByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*Credential, error) {
	result, err := r.queries.GetCredentialByUserID(ctx, sqlcdb.GetCredentialByUserIDParams{
		TenantID: uuidToPgtype(tenantID),
		UserID:   uuidToPgtype(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, platformerrors.NewInfraError("auth.repository.GetCredentialByUserID", err)
	}
	return toCredential(result)
}

func (r *PostgresRepository) UpdatePasswordHash(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error) {
	result, err := r.queries.UpdatePasswordHash(ctx, sqlcdb.UpdatePasswordHashParams{
		PasswordHash: passwordHash,
		TenantID:     uuidToPgtype(tenantID),
		UserID:       uuidToPgtype(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, platformerrors.NewInfraError("auth.repository.UpdatePasswordHash", err)
	}
	return toCredential(result)
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func (r *PostgresRepository) CreateSession(ctx context.Context, session *Session) (*Session, error) {
	result, err := r.queries.CreateSession(ctx, sqlcdb.CreateSessionParams{
		TenantID:         uuidToPgtype(session.TenantID),
		UserID:           uuidToPgtype(session.UserID),
		FamilyID:         uuidToPgtype(session.FamilyID),
		RefreshTokenHash: session.RefreshTokenHash,
		ExpiresAt:        pgtype.Timestamptz{Time: session.ExpiresAt, Valid: true},
		IpAddress:        stringPtrToPgtypeText(session.IPAddress),
		UserAgent:        stringPtrToPgtypeText(session.UserAgent),
	})
	if err != nil {
		return nil, platformerrors.NewInfraError("auth.repository.CreateSession", err)
	}
	return toSession(result)
}

func (r *PostgresRepository) GetSessionByID(ctx context.Context, tenantID, sessionID uuid.UUID) (*Session, error) {
	result, err := r.queries.GetSessionByID(ctx, sqlcdb.GetSessionByIDParams{
		ID:       uuidToPgtype(sessionID),
		TenantID: uuidToPgtype(tenantID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, platformerrors.NewInfraError("auth.repository.GetSessionByID", err)
	}
	return toSession(result)
}

// GetActiveSessionByFamilyIDForUpdate retrieves the active session for a token family
// with a PostgreSQL FOR UPDATE row-level lock.
//
// CONCURRENCY MODEL:
// This method MUST be called within a transaction (via NewPostgresRepositoryWithTx).
// The FOR UPDATE lock ensures that only one concurrent refresh request can proceed
// at a time for a given token family. The second request will block until the first
// commits, then see the updated refresh_token_hash, which won't match its token,
// triggering reuse detection.
func (r *PostgresRepository) GetActiveSessionByFamilyIDForUpdate(ctx context.Context, familyID uuid.UUID) (*Session, error) {
	result, err := r.queries.GetActiveSessionByFamilyID(ctx, uuidToPgtype(familyID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, platformerrors.NewInfraError("auth.repository.GetActiveSessionByFamilyIDForUpdate", err)
	}
	return toSession(result)
}

func (r *PostgresRepository) RotateRefreshToken(ctx context.Context, tenantID, sessionID uuid.UUID, newTokenHash string) (*Session, error) {
	result, err := r.queries.RotateRefreshToken(ctx, sqlcdb.RotateRefreshTokenParams{
		RefreshTokenHash: newTokenHash,
		ID:               uuidToPgtype(sessionID),
		TenantID:         uuidToPgtype(tenantID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, platformerrors.NewInfraError("auth.repository.RotateRefreshToken", err)
	}
	return toSession(result)
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, tenantID, sessionID uuid.UUID) error {
	err := r.queries.RevokeSession(ctx, sqlcdb.RevokeSessionParams{
		ID:       uuidToPgtype(sessionID),
		TenantID: uuidToPgtype(tenantID),
	})
	if err != nil {
		return platformerrors.NewInfraError("auth.repository.RevokeSession", err)
	}
	return nil
}

func (r *PostgresRepository) RevokeAllUserSessions(ctx context.Context, tenantID, userID uuid.UUID) error {
	err := r.queries.RevokeAllUserSessions(ctx, sqlcdb.RevokeAllUserSessionsParams{
		TenantID: uuidToPgtype(tenantID),
		UserID:   uuidToPgtype(userID),
	})
	if err != nil {
		return platformerrors.NewInfraError("auth.repository.RevokeAllUserSessions", err)
	}
	return nil
}

func (r *PostgresRepository) RevokeOtherUserSessions(ctx context.Context, tenantID, userID, currentSessionID uuid.UUID) error {
	err := r.queries.RevokeOtherUserSessions(ctx, sqlcdb.RevokeOtherUserSessionsParams{
		TenantID:  uuidToPgtype(tenantID),
		UserID:    uuidToPgtype(userID),
		SessionID: uuidToPgtype(currentSessionID),
	})
	if err != nil {
		return platformerrors.NewInfraError("auth.repository.RevokeOtherUserSessions", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Password Reset Tokens
// ---------------------------------------------------------------------------

func (r *PostgresRepository) CreatePasswordResetToken(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*PasswordResetToken, error) {
	result, err := r.queries.CreatePasswordResetToken(ctx, sqlcdb.CreatePasswordResetTokenParams{
		TenantID:  uuidToPgtype(tenantID),
		UserID:    uuidToPgtype(userID),
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return nil, platformerrors.NewInfraError("auth.repository.CreatePasswordResetToken", err)
	}
	return toPasswordResetToken(result)
}

func (r *PostgresRepository) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error) {
	result, err := r.queries.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrResetTokenNotFound
		}
		return nil, platformerrors.NewInfraError("auth.repository.GetPasswordResetTokenByHash", err)
	}
	return toPasswordResetToken(result)
}

func (r *PostgresRepository) ConsumePasswordResetToken(ctx context.Context, tokenID uuid.UUID) error {
	err := r.queries.ConsumePasswordResetToken(ctx, uuidToPgtype(tokenID))
	if err != nil {
		return platformerrors.NewInfraError("auth.repository.ConsumePasswordResetToken", err)
	}
	return nil
}

func (r *PostgresRepository) InvalidatePasswordResetTokensForUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	err := r.queries.InvalidatePasswordResetTokensForUser(ctx, sqlcdb.InvalidatePasswordResetTokensForUserParams{
		TenantID: uuidToPgtype(tenantID),
		UserID:   uuidToPgtype(userID),
	})
	if err != nil {
		return platformerrors.NewInfraError("auth.repository.InvalidatePasswordResetTokensForUser", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Email Verification Tokens
// ---------------------------------------------------------------------------

func (r *PostgresRepository) CreateEmailVerificationToken(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*EmailVerificationToken, error) {
	result, err := r.queries.CreateEmailVerificationToken(ctx, sqlcdb.CreateEmailVerificationTokenParams{
		TenantID:  uuidToPgtype(tenantID),
		UserID:    uuidToPgtype(userID),
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return nil, platformerrors.NewInfraError("auth.repository.CreateEmailVerificationToken", err)
	}
	return toEmailVerificationToken(result)
}

func (r *PostgresRepository) GetEmailVerificationTokenByHash(ctx context.Context, tokenHash string) (*EmailVerificationToken, error) {
	result, err := r.queries.GetEmailVerificationTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVerificationTokenNotFound
		}
		return nil, platformerrors.NewInfraError("auth.repository.GetEmailVerificationTokenByHash", err)
	}
	return toEmailVerificationToken(result)
}

func (r *PostgresRepository) ConsumeEmailVerificationToken(ctx context.Context, tokenID uuid.UUID) error {
	err := r.queries.ConsumeEmailVerificationToken(ctx, uuidToPgtype(tokenID))
	if err != nil {
		return platformerrors.NewInfraError("auth.repository.ConsumeEmailVerificationToken", err)
	}
	return nil
}

func (r *PostgresRepository) InvalidateEmailVerificationTokensForUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	err := r.queries.InvalidateEmailVerificationTokensForUser(ctx, sqlcdb.InvalidateEmailVerificationTokensForUserParams{
		TenantID: uuidToPgtype(tenantID),
		UserID:   uuidToPgtype(userID),
	})
	if err != nil {
		return platformerrors.NewInfraError("auth.repository.InvalidateEmailVerificationTokensForUser", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Mapping: sqlcdb types → domain types
// ---------------------------------------------------------------------------

func toCredential(row sqlcdb.UserCredential) (*Credential, error) {
	id, err := uuidFromPgtype(row.ID)
	if err != nil {
		return nil, fmt.Errorf("mapping credential.id: %w", err)
	}
	tenantID, err := uuidFromPgtype(row.TenantID)
	if err != nil {
		return nil, fmt.Errorf("mapping credential.tenant_id: %w", err)
	}
	userID, err := uuidFromPgtype(row.UserID)
	if err != nil {
		return nil, fmt.Errorf("mapping credential.user_id: %w", err)
	}
	return &Credential{
		ID:           id,
		TenantID:     tenantID,
		UserID:       userID,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}

func toSession(row sqlcdb.AuthSession) (*Session, error) {
	id, err := uuidFromPgtype(row.ID)
	if err != nil {
		return nil, fmt.Errorf("mapping session.id: %w", err)
	}
	tenantID, err := uuidFromPgtype(row.TenantID)
	if err != nil {
		return nil, fmt.Errorf("mapping session.tenant_id: %w", err)
	}
	userID, err := uuidFromPgtype(row.UserID)
	if err != nil {
		return nil, fmt.Errorf("mapping session.user_id: %w", err)
	}
	familyID, err := uuidFromPgtype(row.FamilyID)
	if err != nil {
		return nil, fmt.Errorf("mapping session.family_id: %w", err)
	}
	return &Session{
		ID:               id,
		TenantID:         tenantID,
		UserID:           userID,
		FamilyID:         familyID,
		RefreshTokenHash: row.RefreshTokenHash,
		ExpiresAt:        row.ExpiresAt.Time,
		RevokedAt:        timestamptzToTimePtr(row.RevokedAt),
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
		LastUsedAt:       row.LastUsedAt.Time,
		IPAddress:        pgtypeTextToStringPtr(row.IpAddress),
		UserAgent:        pgtypeTextToStringPtr(row.UserAgent),
	}, nil
}

func toPasswordResetToken(row sqlcdb.PasswordResetToken) (*PasswordResetToken, error) {
	id, err := uuidFromPgtype(row.ID)
	if err != nil {
		return nil, fmt.Errorf("mapping password_reset_token.id: %w", err)
	}
	tenantID, err := uuidFromPgtype(row.TenantID)
	if err != nil {
		return nil, fmt.Errorf("mapping password_reset_token.tenant_id: %w", err)
	}
	userID, err := uuidFromPgtype(row.UserID)
	if err != nil {
		return nil, fmt.Errorf("mapping password_reset_token.user_id: %w", err)
	}
	return &PasswordResetToken{
		ID:        id,
		TenantID:  tenantID,
		UserID:    userID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    timestamptzToTimePtr(row.UsedAt),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func toEmailVerificationToken(row sqlcdb.EmailVerificationToken) (*EmailVerificationToken, error) {
	id, err := uuidFromPgtype(row.ID)
	if err != nil {
		return nil, fmt.Errorf("mapping email_verification_token.id: %w", err)
	}
	tenantID, err := uuidFromPgtype(row.TenantID)
	if err != nil {
		return nil, fmt.Errorf("mapping email_verification_token.tenant_id: %w", err)
	}
	userID, err := uuidFromPgtype(row.UserID)
	if err != nil {
		return nil, fmt.Errorf("mapping email_verification_token.user_id: %w", err)
	}
	return &EmailVerificationToken{
		ID:        id,
		TenantID:  tenantID,
		UserID:    userID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    timestamptzToTimePtr(row.UsedAt),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

// ---------------------------------------------------------------------------
// Type conversion helpers: domain ↔ pgtype
// ---------------------------------------------------------------------------

func uuidToPgtype(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}

func uuidFromPgtype(u pgtype.UUID) (uuid.UUID, error) {
	if !u.Valid {
		return uuid.Nil, fmt.Errorf("invalid pgtype.UUID (Valid=false)")
	}
	return uuid.UUID(u.Bytes), nil
}

func timestamptzToTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func stringPtrToPgtypeText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func pgtypeTextToStringPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// ---------------------------------------------------------------------------
// Error classification
// ---------------------------------------------------------------------------

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// ---------------------------------------------------------------------------
// Transaction Manager
// ---------------------------------------------------------------------------

// PostgresTxManager implements TxManager (defined in service.go) using pgxpool.
// It wraps database.WithTx and creates a transaction-scoped Repository for
// the closure, allowing the service layer to execute atomic operations without
// importing pgx directly.
type PostgresTxManager struct {
	pool *pgxpool.Pool
}

// NewPostgresTxManager creates a new PostgresTxManager.
func NewPostgresTxManager(pool *pgxpool.Pool) *PostgresTxManager {
	return &PostgresTxManager{pool: pool}
}

// WithTx executes fn within a database transaction. The fn receives a
// Repository bound to the transaction. If fn returns an error, the
// transaction is rolled back; otherwise it is committed.
func (m *PostgresTxManager) WithTx(ctx context.Context, fn func(txRepo Repository) error) error {
	return database.WithTx(ctx, m.pool, func(tx pgx.Tx) error {
		return fn(NewPostgresRepositoryWithTx(tx))
	})
}
