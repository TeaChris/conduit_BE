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
	"github.com/jackc/pgx/v5/pgxpool"

	platformerrors "github.com/conduit-platform/conduit/backend/internal/platform/errors"
)

// Repository defines the persistence contract for the User domain.
type Repository interface {
	Create(ctx context.Context, user *User) (*User, error)
	GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*User, error)
	List(ctx context.Context, tenantID uuid.UUID, filter ListFilter) ([]User, int64, error)
	Update(ctx context.Context, user *User) (*User, error)
	UpdateStatus(ctx context.Context, tenantID, userID uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error)
	SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) (*User, error)
}

// PostgresRepository implements Repository using PostgreSQL via pgx.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const (
	queryCreateUser = `
		INSERT INTO users (tenant_id, email, display_name, status, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, email, display_name, status, email_verified,
		          email_verified_at, metadata, deactivated_at, created_at, updated_at`

	queryGetUserByID = `
		SELECT id, tenant_id, email, display_name, status, email_verified,
		       email_verified_at, metadata, deactivated_at, created_at, updated_at
		FROM users
		WHERE id = $1 AND tenant_id = $2`

	queryGetUserByEmail = `
		SELECT id, tenant_id, email, display_name, status, email_verified,
		       email_verified_at, metadata, deactivated_at, created_at, updated_at
		FROM users
		WHERE tenant_id = $1 AND lower(email) = lower($2)`

	queryListUsers = `
		SELECT id, tenant_id, email, display_name, status, email_verified,
		       email_verified_at, metadata, deactivated_at, created_at, updated_at
		FROM users
		WHERE tenant_id = $1
		  AND ($2::text IS NULL OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`

	queryCountUsers = `
		SELECT count(*)
		FROM users
		WHERE tenant_id = $1
		  AND ($2::text IS NULL OR status = $2)`

	queryUpdateUser = `
		UPDATE users
		SET email = $1, display_name = $2, metadata = $3,
		    email_verified = $4, email_verified_at = $5,
		    updated_at = now()
		WHERE id = $6 AND tenant_id = $7
		RETURNING id, tenant_id, email, display_name, status, email_verified,
		          email_verified_at, metadata, deactivated_at, created_at, updated_at`

	queryUpdateUserStatus = `
		UPDATE users
		SET status = $1, deactivated_at = $2, updated_at = now()
		WHERE id = $3 AND tenant_id = $4
		RETURNING id, tenant_id, email, display_name, status, email_verified,
		          email_verified_at, metadata, deactivated_at, created_at, updated_at`

	querySetEmailVerified = `
		UPDATE users
		SET email_verified = true, email_verified_at = now(), updated_at = now()
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, email, display_name, status, email_verified,
		          email_verified_at, metadata, deactivated_at, created_at, updated_at`
)

// scanUser scans a single user row into a User domain model.
func scanUser(row pgx.Row) (*User, error) {
	var u User
	var metadata []byte
	var emailVerifiedAt, deactivatedAt *time.Time

	err := row.Scan(
		&u.ID,
		&u.TenantID,
		&u.Email,
		&u.DisplayName,
		&u.Status,
		&u.EmailVerified,
		&emailVerifiedAt,
		&metadata,
		&deactivatedAt,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	u.EmailVerifiedAt = emailVerifiedAt
	u.DeactivatedAt = deactivatedAt

	if metadata != nil {
		if err := json.Unmarshal(metadata, &u.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshaling user metadata: %w", err)
		}
	}
	if u.Metadata == nil {
		u.Metadata = make(map[string]any)
	}

	return &u, nil
}

// scanUsers scans multiple user rows.
func scanUsers(rows pgx.Rows) ([]User, error) {
	users := make([]User, 0)
	for rows.Next() {
		var u User
		var metadata []byte
		var emailVerifiedAt, deactivatedAt *time.Time

		err := rows.Scan(
			&u.ID, &u.TenantID, &u.Email, &u.DisplayName, &u.Status,
			&u.EmailVerified, &emailVerifiedAt, &metadata, &deactivatedAt,
			&u.CreatedAt, &u.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		u.EmailVerifiedAt = emailVerifiedAt
		u.DeactivatedAt = deactivatedAt

		if metadata != nil {
			if err := json.Unmarshal(metadata, &u.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshaling user metadata: %w", err)
			}
		}
		if u.Metadata == nil {
			u.Metadata = make(map[string]any)
		}

		users = append(users, u)
	}
	return users, nil
}

func (r *PostgresRepository) Create(ctx context.Context, user *User) (*User, error) {
	metadata, err := json.Marshal(user.Metadata)
	if err != nil {
		return nil, platformerrors.NewInfraError("user.repository.Create", fmt.Errorf("marshaling metadata: %w", err))
	}

	row := r.pool.QueryRow(ctx, queryCreateUser,
		user.TenantID,
		user.Email,
		user.DisplayName,
		string(user.Status),
		metadata,
	)

	created, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailAlreadyExists
		}
		return nil, platformerrors.NewInfraError("user.repository.Create", err)
	}

	return created, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*User, error) {
	row := r.pool.QueryRow(ctx, queryGetUserByID, userID, tenantID)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.GetByID", err)
	}
	return user, nil
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*User, error) {
	row := r.pool.QueryRow(ctx, queryGetUserByEmail, tenantID, email)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.GetByEmail", err)
	}
	return user, nil
}

func (r *PostgresRepository) List(ctx context.Context, tenantID uuid.UUID, filter ListFilter) ([]User, int64, error) {
	var statusFilter *string
	if filter.Status != nil {
		s := string(*filter.Status)
		statusFilter = &s
	}

	// Count total matching users.
	var total int64
	err := r.pool.QueryRow(ctx, queryCountUsers, tenantID, statusFilter).Scan(&total)
	if err != nil {
		return nil, 0, platformerrors.NewInfraError("user.repository.List.count", err)
	}

	// Fetch the page.
	rows, err := r.pool.Query(ctx, queryListUsers,
		tenantID, statusFilter, filter.PageSize, filter.Offset(),
	)
	if err != nil {
		return nil, 0, platformerrors.NewInfraError("user.repository.List.query", err)
	}
	defer rows.Close()

	users, err := scanUsers(rows)
	if err != nil {
		return nil, 0, platformerrors.NewInfraError("user.repository.List.scan", err)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, platformerrors.NewInfraError("user.repository.List.rows", err)
	}

	return users, total, nil
}

func (r *PostgresRepository) Update(ctx context.Context, user *User) (*User, error) {
	metadata, err := json.Marshal(user.Metadata)
	if err != nil {
		return nil, platformerrors.NewInfraError("user.repository.Update", fmt.Errorf("marshaling metadata: %w", err))
	}

	row := r.pool.QueryRow(ctx, queryUpdateUser,
		user.Email,
		user.DisplayName,
		metadata,
		user.EmailVerified,
		user.EmailVerifiedAt,
		user.ID,
		user.TenantID,
	)

	updated, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		if isUniqueViolation(err) {
			return nil, ErrEmailAlreadyExists
		}
		return nil, platformerrors.NewInfraError("user.repository.Update", err)
	}

	return updated, nil
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, tenantID, userID uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error) {
	row := r.pool.QueryRow(ctx, queryUpdateUserStatus,
		string(status),
		deactivatedAt,
		userID,
		tenantID,
	)

	updated, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.UpdateStatus", err)
	}

	return updated, nil
}

func (r *PostgresRepository) SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) (*User, error) {
	row := r.pool.QueryRow(ctx, querySetEmailVerified, userID, tenantID)
	updated, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, platformerrors.NewInfraError("user.repository.SetEmailVerified", err)
	}
	return updated, nil
}

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
