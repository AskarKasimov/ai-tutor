-- name: InsertUser :exec
INSERT INTO users(id, email, display_name, password_hash, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: EnsureTeacher :one
INSERT INTO users(id, email, display_name, password_hash, role, created_at)
VALUES ('user:shared-teacher', 'shared-teacher@ai-tutor.invalid', 'Учитель', '!', 'admin', $1)
ON CONFLICT (id) DO UPDATE SET role = 'admin'
RETURNING id, email, display_name, role, created_at;

-- name: InsertAuthSession :exec
INSERT INTO auth_sessions(id, user_id, expires_at) VALUES ($1, $2, $3);

-- name: InsertAccessToken :exec
INSERT INTO access_tokens(token_hash, session_id, expires_at) VALUES ($1, $2, $3);

-- name: InsertRefreshToken :exec
INSERT INTO refresh_tokens(token_hash, session_id) VALUES ($1, $2);

-- name: LockRefreshState :one
SELECT s.id, s.expires_at, s.revoked_at, t.used_at
FROM refresh_tokens AS t
JOIN auth_sessions AS s ON s.id = t.session_id
WHERE t.token_hash = $1
FOR UPDATE OF s;

-- name: GetRefreshTokenUsedAt :one
SELECT used_at FROM refresh_tokens WHERE token_hash = $1;

-- name: MarkRefreshTokenUsed :exec
UPDATE refresh_tokens SET used_at = $2 WHERE token_hash = $1;

-- name: RevokeAuthSession :exec
UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, $2) WHERE id = $1;

-- name: RevokeSessionByRefreshToken :exec
UPDATE auth_sessions
SET revoked_at = COALESCE(revoked_at, $2)
WHERE id = (SELECT session_id FROM refresh_tokens WHERE token_hash = $1);

-- name: GetUserForLogin :one
SELECT id, email, display_name, role, created_at, password_hash
FROM users WHERE email = $1;

-- name: GetAuthContextByAccessToken :one
SELECT s.id AS session_id, s.user_id, s.expires_at AS session_expires_at, s.revoked_at,
       t.expires_at AS token_expires_at, u.id AS account_id, u.email, u.display_name, u.role, u.created_at
FROM access_tokens AS t
JOIN auth_sessions AS s ON s.id = t.session_id
JOIN users AS u ON u.id = s.user_id
WHERE t.token_hash = $1;
