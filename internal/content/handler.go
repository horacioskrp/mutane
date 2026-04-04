package content

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

type Handler struct {
	repo *Repository
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{repo: NewRepository(db)}
}

// ── ContentType handlers ──────────────────────────────────────────────────────

func (h *Handler) ListContentTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.repo.ListContentTypes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, types)
}

func (h *Handler) CreateContentType(w http.ResponseWriter, r *http.Request) {
	var ct ContentType
	if err := json.NewDecoder(r.Body).Decode(&ct); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.repo.CreateContentType(r.Context(), &ct); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ct)
}

func (h *Handler) GetContentType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ct, err := h.repo.GetContentType(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ct)
}

func (h *Handler) UpdateContentType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var ct ContentType
	if err := json.NewDecoder(r.Body).Decode(&ct); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ct.ID = id
	if err := h.repo.UpdateContentType(r.Context(), &ct); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ct)
}

func (h *Handler) DeleteContentType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.repo.DeleteContentType(r.Context(), id); err != nil {
		if err.Error() == "cannot delete system content type" {
			writeError(w, http.StatusForbidden, "impossible de supprimer un type système")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Endpoint configuration handlers ──────────────────────────────────────────

// GetEndpointConfig returns the EndpointConfig for a content type.
func (h *Handler) GetEndpointConfig(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ct, err := h.repo.GetContentType(r.Context(), id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ct.EndpointConfig)
}

// UpdateEndpointConfig replaces the EndpointConfig for a content type.
func (h *Handler) UpdateEndpointConfig(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var cfg EndpointConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.repo.UpdateEndpointConfig(r.Context(), id, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// ── Entry handlers ────────────────────────────────────────────────────────────

func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	typeID, err := pathID(r, "typeId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid typeId")
		return
	}
	entries, err := h.repo.ListEntries(r.Context(), typeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	typeID, err := pathID(r, "typeId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid typeId")
		return
	}
	// Accept both {data:{...}} envelope and flat {field:value} body
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var entry Entry
	entry.ContentTypeID = typeID
	if nested, ok := raw["data"].(map[string]any); ok {
		entry.Data = nested
	} else {
		entry.Data = raw
	}
	if err := h.repo.CreateEntry(r.Context(), &entry); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	typeID, err := pathID(r, "typeId")
	if err != nil {
		// fallback: try "id" param (backward compat)
		typeID = 0
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var entry *Entry
	if typeID > 0 {
		entry, err = h.repo.GetEntryForCT(r.Context(), typeID, id)
	} else {
		entry, err = h.repo.GetEntry(r.Context(), id)
	}
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	typeID, _ := pathID(r, "typeId")
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var entry Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	entry.ID = id
	entry.ContentTypeID = typeID
	if err := h.repo.UpdateEntry(r.Context(), &entry); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	typeID, _ := pathID(r, "typeId")
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if typeID > 0 {
		if err := h.repo.DeleteEntryForCT(r.Context(), typeID, id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if err := h.repo.DeleteEntry(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Field handlers ────────────────────────────────────────────────────────────

func (h *Handler) AddField(w http.ResponseWriter, r *http.Request) {
	ctID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var f Field
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	f.ContentTypeID = ctID
	if err := h.repo.AddField(r.Context(), &f); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handler) DeleteField(w http.ResponseWriter, r *http.Request) {
	fid, err := pathID(r, "fid")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if err := h.repo.DeleteField(r.Context(), fid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReorderFields(w http.ResponseWriter, r *http.Request) {
	ctID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		FieldIDs []int64 `json:"field_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.FieldIDs) == 0 {
		writeError(w, http.StatusBadRequest, "field_ids required")
		return
	}
	if err := h.repo.ReorderFields(r.Context(), ctID, body.FieldIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Admin UI page handlers ────────────────────────────────────────────────────

func (h *Handler) DashboardPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}
func (h *Handler) ListPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}
func (h *Handler) NewContentTypePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}
func (h *Handler) BuilderPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}
func (h *Handler) EntryListPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}
func (h *Handler) EntryEditorPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/admin.html")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func pathID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
