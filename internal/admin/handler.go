package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"mutane/internal/ctxkey"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := LoadStats(r.Context(), h.db)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(ctxkey.UserID).(int64)

	var user struct {
		ID      int64  `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		IsAdmin bool   `json:"is_admin"`
	}
	err := h.db.QueryRowContext(r.Context(),
		`SELECT id, email, name, is_admin FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.IsAdmin)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// UpdateMe — PATCH /api/me
// Updates the authenticated user's name and/or email.
func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(ctxkey.UserID).(int64)
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	body.Name  = strings.TrimSpace(body.Name)
	body.Email = strings.TrimSpace(body.Email)
	if body.Email == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "email is required"})
		return
	}

	var user struct {
		ID      int64  `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		IsAdmin bool   `json:"is_admin"`
	}
	err := h.db.QueryRowContext(r.Context(),
		`UPDATE users SET name=$1, email=$2, updated_at=NOW()
		 WHERE id=$3
		 RETURNING id, email, name, is_admin`,
		body.Name, body.Email, userID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.IsAdmin)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(user)
}

// UpdatePassword — PATCH /api/me/password
// Verifies the current password then sets a new bcrypt hash.
func (h *Handler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(ctxkey.UserID).(int64)
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	if len(body.NewPassword) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "new password must be at least 8 characters"})
		return
	}

	// Fetch current hash
	var currentHash string
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT password_hash FROM users WHERE id=$1`, userID,
	).Scan(&currentHash); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		return
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(body.CurrentPassword)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "mot de passe actuel incorrect"})
		return
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "hash error"})
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		`UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`,
		string(newHash), userID,
	); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}

