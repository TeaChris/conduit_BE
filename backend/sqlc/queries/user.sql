-- name: CreateUser :one
INSERT INTO users (tenant_id, email, display_name, status, metadata)
VALUES (@tenant_id, @email, @display_name, @status, @metadata)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE tenant_id = @tenant_id AND lower(email) = lower(@email);

-- name: ListUsers :many
SELECT * FROM users
WHERE tenant_id = @tenant_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('page_size') OFFSET sqlc.arg('page_offset');

-- name: CountUsers :one
SELECT count(*) FROM users
WHERE tenant_id = @tenant_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'));

-- name: UpdateUser :one
UPDATE users
SET email = @email,
    display_name = @display_name,
    metadata = @metadata,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: UpdateUserStatus :one
UPDATE users
SET status = @status,
    deactivated_at = @deactivated_at,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: SetEmailVerified :one
UPDATE users
SET email_verified = true,
    email_verified_at = now(),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;
