// Package ctxkey defines shared context keys used across packages.
// Using a single package avoids the "same string, different type" bug
// where each package defines its own ctxKey type.
package ctxkey

type key string

const UserID key = "userID"
