package public

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

type meta struct {
	Total  int `json:"total"`
	Page   int `json:"page"`
	Limit  int `json:"limit"`
}

type envelope struct {
	Data any  `json:"data"`
	Meta meta `json:"meta"`
}

// GET /v1/{slug}?page=1&limit=20
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	var ctID int64
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT id FROM content_types WHERE slug = $1`, slug,
	).Scan(&ctID); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "content type not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var total int
	h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM entries WHERE content_type_id = $1 AND published_at IS NOT NULL AND published_at <= NOW()`,
		ctID,
	).Scan(&total)

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, content_type_id, data, published_at, created_at, updated_at
		 FROM entries
		 WHERE content_type_id = $1 AND published_at IS NOT NULL AND published_at <= NOW()
		 ORDER BY published_at DESC
		 LIMIT $2 OFFSET $3`,
		ctID, limit, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var entries []map[string]any
	for rows.Next() {
		var (
			id, ctIDv     int64
			data          []byte
			publishedAt   *string
			createdAt     string
			updatedAt     string
		)
		if err := rows.Scan(&id, &ctIDv, &data, &publishedAt, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var fields map[string]any
		json.Unmarshal(data, &fields)
		entry := map[string]any{
			"id":              id,
			"content_type_id": ctIDv,
			"published_at":    publishedAt,
			"created_at":      createdAt,
			"updated_at":      updatedAt,
		}
		for k, v := range fields {
			entry[k] = v
		}
		entries = append(entries, entry)
	}

	writeJSON(w, http.StatusOK, envelope{
		Data: entries,
		Meta: meta{Total: total, Page: page, Limit: limit},
	})
}

// GET /v1/{slug}/{id}
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var ctID int64
	if err := h.db.QueryRowContext(r.Context(),
		`SELECT id FROM content_types WHERE slug = $1`, slug,
	).Scan(&ctID); err != nil {
		writeError(w, http.StatusNotFound, "content type not found")
		return
	}

	var (
		data        []byte
		publishedAt *string
		createdAt   string
		updatedAt   string
	)
	err = h.db.QueryRowContext(r.Context(),
		`SELECT data, published_at, created_at, updated_at
		 FROM entries
		 WHERE id = $1 AND content_type_id = $2 AND published_at IS NOT NULL AND published_at <= NOW()`,
		id, ctID,
	).Scan(&data, &publishedAt, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var fields map[string]any
	json.Unmarshal(data, &fields)
	entry := map[string]any{
		"id":              id,
		"content_type_id": ctID,
		"published_at":    publishedAt,
		"created_at":      createdAt,
		"updated_at":      updatedAt,
	}
	for k, v := range fields {
		entry[k] = v
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entry})
}

// GET /v1/media?page=1&limit=50
func (h *Handler) ListMedia(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 50)
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	var total int
	h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media`).Scan(&total)

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, filename, original_name, mime_type, size, url, created_at FROM media ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var (
			id                         int64
			filename, originalName     string
			mimeType                   string
			size                       int64
			url, createdAt             string
		)
		rows.Scan(&id, &filename, &originalName, &mimeType, &size, &url, &createdAt)
		items = append(items, map[string]any{
			"id": id, "filename": filename, "original_name": originalName,
			"mime_type": mimeType, "size": size, "url": url, "created_at": createdAt,
		})
	}

	writeJSON(w, http.StatusOK, envelope{
		Data: items,
		Meta: meta{Total: total, Page: page, Limit: limit},
	})
}

func queryInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || v < 1 {
		return def
	}
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
