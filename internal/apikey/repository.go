package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Repository handles all API key persistence.
// Keys are stored as SHA-256 hashes (fast lookup, collision-safe for 32-byte random tokens).
// The full plaintext is only ever held in memory and returned once on creation/rotation.
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// generatePlaintext builds a cryptographically random key:
//   public  → "mut_pub_<64 hex chars>"
//   private → "mut_prv_<64 hex chars>"
func generatePlaintext(kt KeyType) (plaintext, prefix string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate key bytes: %w", err)
	}
	body := hex.EncodeToString(raw) // 64 chars

	var p string
	switch kt {
	case KeyTypePrivate:
		p = rawPrefixPrivate
	default:
		p = rawPrefixPublic
	}

	plaintext = p + body
	// prefix = first 12 chars (e.g. "mut_pub_ab12") — safe to store/display
	prefix = plaintext[:prefixDisplayLen]
	return plaintext, prefix, nil
}

// hashKey returns the hex-encoded SHA-256 of the plaintext key.
// We use SHA-256 (not bcrypt) because:
//   - keys are 32 bytes of crypto/rand — already high-entropy, no need for KDF stretching
//   - SHA-256 is O(1) vs O(hundreds ms) for bcrypt, which matters for every API request
func hashKey(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// CreateOptions bundles optional parameters for key creation.
type CreateOptions struct {
	Description string
	Type        KeyType
	ExpiresAt   *time.Time
}

// Create generates a new API key, persists its SHA-256 hash, and returns the
// plaintext value (shown to the user exactly once).
func (r *Repository) Create(ctx context.Context, userID int64, name string, opts CreateOptions) (key *APIKey, plaintext string, err error) {
	kt := opts.Type
	if kt == "" {
		kt = KeyTypePublic
	}

	plaintext, prefix, err := generatePlaintext(kt)
	if err != nil {
		return nil, "", err
	}

	hash := hashKey(plaintext)

	key = &APIKey{
		UserID:      userID,
		Name:        name,
		Description: opts.Description,
		Type:        kt,
		Prefix:      prefix,
		ExpiresAt:   opts.ExpiresAt,
	}

	err = r.db.QueryRowContext(ctx, `
		INSERT INTO api_keys (user_id, name, description, type, key_hash, prefix, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		userID, name, opts.Description, string(kt), hash, prefix, opts.ExpiresAt,
	).Scan(&key.ID, &key.CreatedAt)
	return key, plaintext, err
}

// Rotate generates a replacement key for an existing one.
// The old key is soft-revoked atomically; the new key stores rotated_from_id.
// Returns the new key and its plaintext.
func (r *Repository) Rotate(ctx context.Context, oldID, userID int64) (newKey *APIKey, plaintext string, err error) {
	// 1. Fetch the old key to clone its properties
	old, err := r.getByID(ctx, oldID, userID)
	if err != nil {
		return nil, "", fmt.Errorf("old key not found: %w", err)
	}
	if !old.IsActive() {
		return nil, "", fmt.Errorf("key is already revoked or expired")
	}

	plaintext, prefix, err := generatePlaintext(old.Type)
	if err != nil {
		return nil, "", err
	}
	hash := hashKey(plaintext)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	// 2. Soft-revoke the old key
	if _, err = tx.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		oldID, userID,
	); err != nil {
		return nil, "", fmt.Errorf("revoke old key: %w", err)
	}

	// 3. Insert the new key
	newKey = &APIKey{
		UserID:        userID,
		Name:          old.Name + " (rotation)",
		Description:   old.Description,
		Type:          old.Type,
		Prefix:        prefix,
		ExpiresAt:     old.ExpiresAt,
		RotatedFromID: &oldID,
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO api_keys (user_id, name, description, type, key_hash, prefix, expires_at, rotated_from_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		userID, newKey.Name, newKey.Description, string(newKey.Type),
		hash, prefix, newKey.ExpiresAt, oldID,
	).Scan(&newKey.ID, &newKey.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("insert new key: %w", err)
	}

	return newKey, plaintext, tx.Commit()
}

// List returns all API keys for a user, ordered newest first.
// Revoked and expired keys are included so the user can see history.
func (r *Repository) List(ctx context.Context, userID int64) ([]APIKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, type, prefix,
		       expires_at, revoked_at, rotated_from_id, last_used_at, created_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(
			&k.ID, &k.UserID, &k.Name, &k.Description, &k.Type, &k.Prefix,
			&k.ExpiresAt, &k.RevokedAt, &k.RotatedFromID, &k.LastUsedAt, &k.CreatedAt,
		); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Revoke soft-deletes an API key (sets revoked_at).
// Only the owning user can revoke their own keys.
func (r *Repository) Revoke(ctx context.Context, id, userID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Validate checks a plaintext key, ensures it is active (not revoked, not expired),
// and updates last_used_at on success.
func (r *Repository) Validate(ctx context.Context, plaintext string) (*APIKey, error) {
	if len(plaintext) < prefixDisplayLen {
		return nil, fmt.Errorf("invalid key format")
	}
	prefix := plaintext[:prefixDisplayLen]
	hash := hashKey(plaintext)

	var k APIKey
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, description, type, prefix,
		       expires_at, revoked_at, rotated_from_id, last_used_at, created_at
		FROM api_keys
		WHERE prefix = $1 AND key_hash = $2`,
		prefix, hash,
	).Scan(
		&k.ID, &k.UserID, &k.Name, &k.Description, &k.Type, &k.Prefix,
		&k.ExpiresAt, &k.RevokedAt, &k.RotatedFromID, &k.LastUsedAt, &k.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid api key")
	}
	if err != nil {
		return nil, err
	}

	if !k.IsActive() {
		return nil, fmt.Errorf("api key is revoked or expired")
	}

	_, _ = r.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, k.ID)
	return &k, nil
}

// ── internal ──────────────────────────────────────────────────────────────────

func (r *Repository) getByID(ctx context.Context, id, userID int64) (*APIKey, error) {
	var k APIKey
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, description, type, prefix,
		       expires_at, revoked_at, rotated_from_id, last_used_at, created_at
		FROM api_keys WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(
		&k.ID, &k.UserID, &k.Name, &k.Description, &k.Type, &k.Prefix,
		&k.ExpiresAt, &k.RevokedAt, &k.RotatedFromID, &k.LastUsedAt, &k.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &k, nil
}
