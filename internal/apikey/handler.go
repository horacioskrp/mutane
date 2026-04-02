package apikey

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"mutane/internal/ctxkey"
)

type Handler struct {
	repo *Repository
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{repo: NewRepository(db)}
}

// List returns all keys (active + revoked) for the authenticated user.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := sessionUserID(r)
	keys, err := h.repo.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []APIKey{}
	}
	writeJSON(w, http.StatusOK, keys)
}

// Create generates a new API key and returns the plaintext once.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Type        KeyType `json:"type"`
		// ExpiresIn: optional number of days until expiry (0 = never)
		ExpiresInDays int `json:"expires_in_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	opts := CreateOptions{
		Description: body.Description,
		Type:        body.Type,
	}
	if body.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, body.ExpiresInDays)
		opts.ExpiresAt = &t
	}

	userID := sessionUserID(r)
	if userID == 0 {
		log.Println("[apikey] Create: userID is 0 — missing or invalid Bearer token")
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	key, plaintext, err := h.repo.Create(r.Context(), userID, body.Name, opts)
	if err != nil {
		log.Printf("[apikey] Create error (userID=%d): %v", userID, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"key":       key,
		"plaintext": plaintext,
	})
}

// Rotate soft-revokes an existing key and creates a replacement with the same settings.
// Returns the new key + its plaintext (shown once).
func (h *Handler) Rotate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	userID := sessionUserID(r)
	newKey, plaintext, err := h.repo.Rotate(r.Context(), id, userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"key":       newKey,
		"plaintext": plaintext,
	})
}

// Revoke soft-deletes a key (sets revoked_at).
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	userID := sessionUserID(r)
	if err := h.repo.Revoke(r.Context(), id, userID); err != nil {
		writeError(w, http.StatusNotFound, "key not found or already revoked")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) KeysPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}

// ── helpers ───────────────────────────────────────────────────────────────────

// sessionUserID extracts the authenticated user ID from the request context.
// Works with both BearerAuth and SessionAuth middlewares (both use ctxkey.UserID).
func sessionUserID(r *http.Request) int64 {
	v, _ := r.Context().Value(ctxkey.UserID).(int64)
	return v
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
