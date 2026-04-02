package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"

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

func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}

