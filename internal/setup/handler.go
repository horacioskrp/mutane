package setup

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// Status godoc
// GET /api/setup/status
// Returns {"initialized": true/false}
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"initialized": count > 0})
}

// Init godoc
// POST /api/setup
// Creates the first admin user. Returns 409 if already initialized.
func (h *Handler) Init(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "platform already initialized")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name, email and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var userID int64
	if err := h.db.QueryRowContext(r.Context(),
		`INSERT INTO users (name, email, password_hash, is_admin) VALUES ($1, $2, $3, true) RETURNING id`,
		req.Name, req.Email, string(hash),
	).Scan(&userID); err != nil {
		writeError(w, http.StatusConflict, "email already exists")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    userID,
		"email": req.Email,
		"name":  req.Name,
	})
}

// SetupPage serves the onboarding HTML.
func (h *Handler) SetupPage(w http.ResponseWriter, r *http.Request) {
	// If already initialized, redirect to login
	var count int
	h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&count)
	if count > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.ServeFile(w, r, "web/static/setup.html")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
