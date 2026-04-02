package apikey

import "time"

// KeyType distinguishes public (browser-safe) from private (server-to-server) keys.
type KeyType string

const (
	KeyTypePublic  KeyType = "public"
	KeyTypePrivate KeyType = "private"
)

// PrefixPublic  = "mut_pub_" + 8 hex chars
// PrefixPrivate = "mut_prv_" + 8 hex chars
const (
	rawPrefixPublic  = "mut_pub_"
	rawPrefixPrivate = "mut_prv_"
	prefixDisplayLen = 12 // "mut_pub_" (8) + 4 chars
)

// APIKey is the safe representation returned to the client (no hash, no plaintext).
type APIKey struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Type           KeyType    `json:"type"`
	Prefix         string     `json:"prefix"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	RotatedFromID  *int64     `json:"rotated_from_id,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// IsActive returns true when the key has not been revoked and is not expired.
func (k *APIKey) IsActive() bool {
	if k.RevokedAt != nil {
		return false
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return false
	}
	return true
}
