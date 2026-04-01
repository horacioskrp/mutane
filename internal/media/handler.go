package media

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

const maxUploadSize = 32 << 20 // 32 MB

type Handler struct {
	storage Storage
	repo    *Repository
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{
		storage: NewLocalStorage(),
		repo:    NewRepository(db),
	}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or bad form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	filename, err := h.storage.Save(header.Filename, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	userID, _ := r.Context().Value(userIDKey).(int64)
	var uid *int64
	if userID != 0 {
		uid = &userID
	}

	m := &Media{
		Filename:     filename,
		OriginalName: header.Filename,
		MimeType:     header.Header.Get("Content-Type"),
		Size:         header.Size,
		URL:          h.storage.URL(filename),
		UploadedBy:   uid,
	}
	if err := h.repo.Create(r.Context(), m); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	items, total, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  items,
		"total": total,
	})
}

func (h *Handler) ManagerPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	filename, err := h.repo.Delete(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = h.storage.Delete(filename)
	w.WriteHeader(http.StatusNoContent)
}

type ctxKey string

const userIDKey ctxKey = "userID"

func queryInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
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
