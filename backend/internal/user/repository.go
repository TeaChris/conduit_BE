package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/conduit-platform/conduit/backend/internal/platform/database/sqlcdb"
	platformerrors "github.com/conduit-platform/conduit/backend/internal/platform/errors"
)

// Repository defines the persistence contract for the User domain.
// The application/service layer depends on this interface, not on PostgreSQL or sqlc.
type Repository interface {
	Create(ctx context.Context, user *User) (*User, error)
	GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*User, error)
	List(ctx context.Context, tenantID uuid.UUID, filter ListFilter) ([]User, int64, error)
	Update(ctx context.Context, user *User) (*User, error)
	UpdateStatus(ctx context.Context, tenantID, userID uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error)
	SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) (*User, error)
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

// ---------------------------------------------------------------------------
// Repository methods
// ---------------------------------------------------------------------------

func (r *PostgresRepository) Create(ctx context.Context, user *User) (*User, error) {
	metadata, err := marshalMetadata(user.Metadata)
	if err != nil {
		return nil, platformerrors.NewInfraError("user.repository.Create", err)
	}

	result, err := r.queries.CreateUser(ctx, sqlcdb.CreateUserParams{
		TenantID:    uuidToPgtype(user.TenantID),
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Status:      string(user.Status),
		Metadata:    metadata,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailAlreadyExists
		}
		return nil, platformerrors.NewInfraError("user.repository.Create", err)
	}

	return toDomainUser(result)
}

func (r *PostgresRepository) GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*User, error) {
	result, err := r.queries.GetUserByID(ctx, sqlcdb.GetUserByIDParams{
		ID:       uuidToPgtype(userID),
		TenantID: uuidToPgtype(tenantID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.GetByID", err)
	}

	return toDomainUser(result)
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*User, error) {
	result, err := r.queries.GetUserByEmail(ctx, sqlcdb.GetUserByEmailParams{
		TenantID: uuidToPgtype(tenantID),
		Email:    email,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.GetByEmail", err)
	}

	return toDomainUser(result)
}

func (r *PostgresRepository) List(ctx context.Context, tenantID uuid.UUID, filter ListFilter) ([]User, int64, error) {
	pgTenantID := uuidToPgtype(tenantID)
	statusFilter := statusToPgtypeText(filter.Status)

	total, err := r.queries.CountUsers(ctx, sqlcdb.CountUsersParams{
		TenantID: pgTenantID,
		Status:   statusFilter,
	})
	if err != nil {
		return nil, 0, platformerrors.NewInfraError("user.repository.List.count", err)
	}

	rows, err := r.queries.ListUsers(ctx, sqlcdb.ListUsersParams{
		TenantID:   pgTenantID,
		Status:     statusFilter,
		PageSize:   int32(filter.PageSize),
		PageOffset: int32(filter.Offset()),
	})
	if err != nil {
		return nil, 0, platformerrors.NewInfraError("user.repository.List.query", err)
	}

	users := make([]User, 0, len(rows))
	for _, row := range rows {
		u, err := toDomainUser(row)
		if err != nil {
			return nil, 0, platformerrors.NewInfraError("user.repository.List.map", err)
		}
		users = append(users, *u)
	}

	return users, total, nil
}

func (r *PostgresRepository) Update(ctx context.Context, user *User) (*User, error) {
	metadata, err := marshalMetadata(user.Metadata)
	if err != nil {
		return nil, platformerrors.NewInfraError("user.repository.Update", err)
	}

	result, err := r.queries.UpdateUser(ctx, sqlcdb.UpdateUserParams{
		Email:           user.Email,
		DisplayName:     user.DisplayName,
		Metadata:        metadata,
		EmailVerified:   user.EmailVerified,
		EmailVerifiedAt: timePtrToTimestamptz(user.EmailVerifiedAt),
		ID:              uuidToPgtype(user.ID),
		TenantID:        uuidToPgtype(user.TenantID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		if isUniqueViolation(err) {
			return nil, ErrEmailAlreadyExists
		}
		return nil, platformerrors.NewInfraError("user.repository.Update", err)
	}

	return toDomainUser(result)
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, tenantID, userID uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error) {
	result, err := r.queries.UpdateUserStatus(ctx, sqlcdb.UpdateUserStatusParams{
		Status:        string(status),
		DeactivatedAt: timePtrToTimestamptz(deactivatedAt),
		ID:            uuidToPgtype(userID),
		TenantID:      uuidToPgtype(tenantID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.UpdateStatus", err)
	}

	return toDomainUser(result)
}

func (r *PostgresRepository) SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) (*User, error) {
	result, err := r.queries.SetEmailVerified(ctx, sqlcdb.SetEmailVerifiedParams{
		ID:       uuidToPgtype(userID),
		TenantID: uuidToPgtype(tenantID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.SetEmailVerified", err)
	}

	return toDomainUser(result)
}

// ---------------------------------------------------------------------------
// Mapping: sqlcdb.User → domain User
// ---------------------------------------------------------------------------

// toDomainUser converts a sqlc-generated User to a domain User.
// Returns an error only if the database returned structurally invalid data,
// which indicates a data integrity problem that must not be hidden.
func toDomainUser(row sqlcdb.User) (*User, error) {
	id, err := uuidFromPgtype(row.ID)
	if err != nil {
		return nil, fmt.Errorf("mapping user.id: %w", err)
	}
	tenantID, err := uuidFromPgtype(row.TenantID)
	if err != nil {
		return nil, fmt.Errorf("mapping user.tenant_id: %w", err)
	}

	metadata, err := unmarshalMetadata(row.Metadata)
	if err != nil {
		return nil, fmt.Errorf("mapping user.metadata: %w", err)
	}

	return &User{
		ID:              id,
		TenantID:        tenantID,
		Email:           row.Email,
		DisplayName:     row.DisplayName,
		Status:          Status(row.Status),
		EmailVerified:   row.EmailVerified,
		EmailVerifiedAt: timestamptzToTimePtr(row.EmailVerifiedAt),
		Metadata:        metadata,
		DeactivatedAt:   timestamptzToTimePtr(row.DeactivatedAt),
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
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

func timePtrToTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func statusToPgtypeText(s *Status) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: string(*s), Valid: true}
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func marshalMetadata(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func unmarshalMetadata(data []byte) (map[string]any, error) {
	if data == nil {
		return make(map[string]any), nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return make(map[string]any), nil
	}
	return m, nil
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
