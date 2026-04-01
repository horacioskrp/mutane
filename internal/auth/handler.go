package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var userID int64
	err = h.db.QueryRowContext(r.Context(),
		`INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id`,
		req.Email, string(hash), req.Name,
	).Scan(&userID)
	if err != nil {
		writeError(w, http.StatusConflict, "email already exists")
		return
	}

	token, err := GenerateToken(userID, req.Email, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var userID int64
	var hash, email string
	var isAdmin bool
	var totpSecret sql.NullString

	err := h.db.QueryRowContext(r.Context(),
		`SELECT id, email, password_hash, is_admin, totp_secret FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &email, &hash, &isAdmin, &totpSecret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if totpSecret.Valid && totpSecret.String != "" {
		if req.TOTPCode == "" {
			writeJSON(w, http.StatusOK, map[string]bool{"totp_required": true})
			return
		}
		if !ValidateTOTP(totpSecret.String, req.TOTPCode) {
			writeError(w, http.StatusUnauthorized, "invalid 2FA code")
			return
		}
	}

	token, err := GenerateToken(userID, email, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"token":    token,
		"redirect": "/admin/",
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) Enable2FA(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(userIDCtxKey).(int64)
	if userID == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var email string
	if err := h.db.QueryRowContext(r.Context(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	secret, err := GenerateTOTPSecret(email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := h.db.ExecContext(r.Context(), `UPDATE users SET totp_secret = $1 WHERE id = $2`, secret.Secret, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": secret.URL, "secret": secret.Secret})
}

func (h *Handler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID, _ := r.Context().Value(userIDCtxKey).(int64)
	if userID == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var secret string
	if err := h.db.QueryRowContext(r.Context(), `SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&secret); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if !ValidateTOTP(secret, req.Code) {
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"verified": true})
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_token"); err == nil && cookie.Value != "" {
		if _, err := ValidateToken(cookie.Value); err == nil {
			http.Redirect(w, r, "/admin/", http.StatusSeeOther)
			return
		}
	}
	http.ServeFile(w, r, "web/static/login.html")
}

type ctxKey string

const userIDCtxKey ctxKey = "userID"

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
