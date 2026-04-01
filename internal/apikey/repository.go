package apikey

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const keyPrefix = "mut_"

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Create generates a new API key, stores its hash, and returns the plaintext key (shown once).
func (r *Repository) Create(ctx context.Context, userID int64, name string) (key *APIKey, plaintext string, err error) {
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		return nil, "", fmt.Errorf("generate key: %w", err)
	}
	plaintext = keyPrefix + hex.EncodeToString(raw)
	prefix := plaintext[:len(keyPrefix)+8] // "mut_" + first 8 chars

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("hash key: %w", err)
	}

	key = &APIKey{UserID: userID, Name: name, Prefix: prefix}
	err = r.db.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, name, key_hash, prefix) VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		userID, name, string(hash), prefix,
	).Scan(&key.ID, &key.CreatedAt)
	return key, plaintext, err
}

// List returns all API keys for a user (no plaintext).
func (r *Repository) List(ctx context.Context, userID int64) ([]APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, prefix, last_used_at, created_at FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.LastUsedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Revoke deletes an API key owned by the given user.
func (r *Repository) Revoke(ctx context.Context, id, userID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = $1 AND user_id = $2`, id, userID,
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

// Validate checks a plaintext key against stored hashes.
// On success it updates last_used_at and returns the matching APIKey.
func (r *Repository) Validate(ctx context.Context, plaintext string) (*APIKey, error) {
	if len(plaintext) < len(keyPrefix)+8 {
		return nil, fmt.Errorf("invalid key format")
	}
	prefix := plaintext[:len(keyPrefix)+8]

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, prefix, key_hash, last_used_at, created_at FROM api_keys WHERE prefix = $1`,
		prefix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var k APIKey
		var hash string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &hash, &k.LastUsedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil {
			_, _ = r.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, k.ID)
			return &k, nil
		}
	}
	return nil, fmt.Errorf("invalid api key")
}
