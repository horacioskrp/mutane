// Package public serves the /v1/* API routes.  Every route respects the
// EndpointConfig stored on the content_type row:
//
//   - Public: true  → no authentication required
//   - Public: false → valid X-API-Key header (or ?api_key=) required
//   - Methods map   → individual HTTP verbs can be disabled per collection
//   - Features map  → per-method opt-in capabilities (pagination, filters, …)
package public

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// ── Response helpers ──────────────────────────────────────────────────────────

type meta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type envelope struct {
	Data any  `json:"data"`
	Meta meta `json:"meta"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── Endpoint config loader ────────────────────────────────────────────────────

// endpointConfig mirrors content.EndpointConfig — duplicated here to avoid an
// import cycle.  Values are unmarshalled directly from the JSONB column.
type endpointConfig struct {
	Public   bool                       `json:"public"`
	Methods  map[string]bool            `json:"methods"`
	Features map[string]map[string]bool `json:"features"`
}

type ctInfo struct {
	ID       int64
	Slug     string
	IsSystem bool
	Config   endpointConfig
}

// loadCTInfo fetches id, is_system, and endpoint_config for the given slug.
// Returns (nil, nil) when the slug does not exist.
func (h *Handler) loadCTInfo(ctx context.Context, slug string) (*ctInfo, error) {
	info := &ctInfo{Slug: slug}
	var raw []byte
	err := h.db.QueryRowContext(ctx,
		`SELECT id, is_system, endpoint_config FROM content_types WHERE slug=$1`,
		slug,
	).Scan(&info.ID, &info.IsSystem, &raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Provide safe defaults before unmarshalling
	info.Config.Methods = map[string]bool{
		"find": true, "findOne": true, "create": true, "update": true, "delete": true,
	}
	info.Config.Features = map[string]map[string]bool{
		"find": {"pagination": true},
	}
	if len(raw) > 2 {
		_ = json.Unmarshal(raw, &info.Config)
	}
	return info, nil
}

// ── Authentication ────────────────────────────────────────────────────────────

// validateAPIKey checks X-API-Key header (or ?api_key= query param) against
// the api_keys table using SHA-256 comparison.  Updates last_used_at
// asynchronously on success.
func (h *Handler) validateAPIKey(r *http.Request) bool {
	key := r.Header.Get("X-API-Key")
	if key == "" {
		key = r.URL.Query().Get("api_key")
	}
	if len(key) < 12 {
		return false
	}
	prefix := key[:12]
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])

	var id int64
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id FROM api_keys
		WHERE prefix=$1 AND key_hash=$2
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, prefix, hash).Scan(&id)
	if err == nil {
		go h.db.Exec(`UPDATE api_keys SET last_used_at=NOW() WHERE id=$1`, id) //nolint:errcheck
	}
	return err == nil
}

// requireAuth writes a 401 and returns false when auth is missing/invalid.
func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request, cfg endpointConfig) bool {
	if cfg.Public {
		return true
	}
	if !h.validateAPIKey(r) {
		writeError(w, http.StatusUnauthorized,
			"authentication required — provide a valid X-API-Key header or ?api_key= query param")
		return false
	}
	return true
}

// ── Common query helpers ──────────────────────────────────────────────────────

func queryInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || v < 1 {
		return def
	}
	return v
}

// sanitizeIdentifier strips everything except [a-zA-Z0-9_] to prevent
// SQL injection in dynamically-constructed ORDER BY / JSONB key paths.
func sanitizeIdentifier(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// buildSort returns a safe ORDER BY fragment from the ?sort=field:asc|desc
// query param.  Unknown/unsafe fields fall back to the provided default.
//
// Only columns that match [a-zA-Z0-9_] pass; bare meta-columns
// (created_at, updated_at, published_at) are compared directly; everything
// else is wrapped in data->>'field'.
func buildSort(r *http.Request, fallback string, features map[string]bool, allowMeta []string) string {
	if !features["sort"] {
		return fallback
	}
	s := r.URL.Query().Get("sort")
	if s == "" {
		return fallback
	}
	parts := strings.SplitN(s, ":", 2)
	field := sanitizeIdentifier(parts[0])
	if field == "" {
		return fallback
	}
	dir := "ASC"
	if len(parts) == 2 && strings.EqualFold(parts[1], "desc") {
		dir = "DESC"
	}
	for _, m := range allowMeta {
		if field == m {
			return field + " " + dir
		}
	}
	return fmt.Sprintf("data->>'%s' %s", field, dir)
}

// ── User CT (system) ──────────────────────────────────────────────────────────

// userSafeFields is the fixed set of columns returned for User CT queries.
// password_hash and totp_secret are intentionally omitted.
const userSelect = `SELECT id, name, email, is_admin, data, created_at::text, updated_at::text FROM users`

func marshalUser(id int64, name, email string, isAdmin bool, dataRaw []byte, createdAt, updatedAt string) map[string]any {
	u := map[string]any{
		"id": id, "name": name, "email": email,
		"is_admin": isAdmin, "created_at": createdAt, "updated_at": updatedAt,
	}
	var custom map[string]any
	if json.Unmarshal(dataRaw, &custom) == nil {
		for k, v := range custom {
			u[k] = v
		}
	}
	return u
}

func (h *Handler) buildUserConditions(r *http.Request, features map[string]bool) (string, []any) {
	var conds []string
	var args []any

	if features["filters"] {
		allowed := map[string]bool{"name": true, "email": true, "is_admin": true}
		for k, v := range r.URL.Query() {
			if allowed[k] {
				args = append(args, v[0])
				conds = append(conds, fmt.Sprintf("%s=$%d", k, len(args)))
			}
		}
	}
	if features["search"] {
		if q := r.URL.Query().Get("search"); q != "" {
			args = append(args, "%"+strings.ToLower(q)+"%")
			n := len(args)
			conds = append(conds, fmt.Sprintf("(LOWER(name) LIKE $%d OR LOWER(email) LIKE $%d)", n, n))
		}
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (h *Handler) listUsers(ctx context.Context, page, limit int, features map[string]bool, r *http.Request) ([]map[string]any, int, error) {
	where, args := h.buildUserConditions(r, features)
	offset := (page - 1) * limit

	var total int
	h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`+where, args...).Scan(&total) //nolint:errcheck

	metaCols := []string{"created_at", "updated_at", "name", "email"}
	orderBy := buildSort(r, "created_at DESC", features, metaCols)

	lIdx := len(args) + 1
	oIdx := len(args) + 2
	rows, err := h.db.QueryContext(ctx, fmt.Sprintf(
		userSelect+"%s ORDER BY %s LIMIT $%d OFFSET $%d",
		where, orderBy, lIdx, oIdx,
	), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var id int64
		var name, email, createdAt, updatedAt string
		var isAdmin bool
		var dataRaw []byte
		if err := rows.Scan(&id, &name, &email, &isAdmin, &dataRaw, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, marshalUser(id, name, email, isAdmin, dataRaw, createdAt, updatedAt))
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result, total, rows.Err()
}

func (h *Handler) getUser(ctx context.Context, id int64) (map[string]any, error) {
	var name, email, createdAt, updatedAt string
	var isAdmin bool
	var dataRaw []byte
	err := h.db.QueryRowContext(ctx,
		userSelect+` WHERE id=$1`, id,
	).Scan(&id, &name, &email, &isAdmin, &dataRaw, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return marshalUser(id, name, email, isAdmin, dataRaw, createdAt, updatedAt), nil
}

// ── Entry CT ──────────────────────────────────────────────────────────────────

func (h *Handler) buildEntryConditions(r *http.Request, ctID int64, features map[string]bool) (string, []any) {
	// arg $1 is always ctID
	args := []any{ctID}
	conds := []string{
		"content_type_id=$1",
		"published_at IS NOT NULL AND published_at <= NOW()",
	}

	if features["filters"] {
		// ?filters[field]=value → data->>'field' = value
		for key, values := range r.URL.Query() {
			if strings.HasPrefix(key, "filters[") && strings.HasSuffix(key, "]") {
				field := sanitizeIdentifier(key[8 : len(key)-1])
				if field != "" && len(values) > 0 {
					args = append(args, values[0])
					conds = append(conds, fmt.Sprintf("data->>'%s'=$%d", field, len(args)))
				}
			}
		}
	}

	if features["search"] {
		if q := r.URL.Query().Get("search"); q != "" {
			args = append(args, "%"+q+"%")
			conds = append(conds, fmt.Sprintf("data::text ILIKE $%d", len(args)))
		}
	}

	return "WHERE " + strings.Join(conds, " AND "), args
}

func marshalEntry(id, ctIDv int64, data []byte, publishedAt *string, createdAt, updatedAt string) map[string]any {
	e := map[string]any{
		"id": id, "content_type_id": ctIDv,
		"published_at": publishedAt, "created_at": createdAt, "updated_at": updatedAt,
	}
	var fields map[string]any
	if json.Unmarshal(data, &fields) == nil {
		for k, v := range fields {
			e[k] = v
		}
	}
	return e
}

// ═══════════════════════════════════════════════════════════════════════════
//  PUBLIC ROUTE HANDLERS
// ═══════════════════════════════════════════════════════════════════════════

// GET /v1/{slug}?page=1&limit=20[&sort=field:asc][&search=q][&filters[x]=y]
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	info, err := h.loadCTInfo(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info == nil {
		writeError(w, http.StatusNotFound, "content type not found")
		return
	}
	if !h.requireAuth(w, r, info.Config) {
		return
	}
	if !info.Config.Methods["find"] {
		writeError(w, http.StatusMethodNotAllowed, "find method not enabled for this endpoint")
		return
	}

	features := info.Config.Features["find"]
	if features == nil {
		features = map[string]bool{"pagination": true}
	}

	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	if !features["pagination"] {
		page, limit = 1, 1000
	}
	offset := (page - 1) * limit

	// ── User CT (system) ─────────────────────────────────────────────────
	if info.IsSystem && slug == "users" {
		users, total, err := h.listUsers(r.Context(), page, limit, features, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: users, Meta: meta{Total: total, Page: page, Limit: limit}})
		return
	}

	// ── Regular CT ───────────────────────────────────────────────────────
	whereClause, args := h.buildEntryConditions(r, info.ID, features)
	metaCols := []string{"created_at", "updated_at", "published_at"}
	orderBy := buildSort(r, "published_at DESC", features, metaCols)

	lIdx := len(args) + 1
	oIdx := len(args) + 2

	var total int
	h.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM entries `+whereClause, args...).Scan(&total) //nolint:errcheck

	rows, err := h.db.QueryContext(r.Context(), fmt.Sprintf(
		`SELECT id, content_type_id, data, published_at::text, created_at::text, updated_at::text
		 FROM entries %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, lIdx, oIdx,
	), append(args, limit, offset)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	entries := make([]map[string]any, 0)
	for rows.Next() {
		var id, ctIDv int64
		var data []byte
		var publishedAt *string
		var createdAt, updatedAt string
		if err := rows.Scan(&id, &ctIDv, &data, &publishedAt, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		entries = append(entries, marshalEntry(id, ctIDv, data, publishedAt, createdAt, updatedAt))
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

	info, err := h.loadCTInfo(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info == nil {
		writeError(w, http.StatusNotFound, "content type not found")
		return
	}
	if !h.requireAuth(w, r, info.Config) {
		return
	}
	if !info.Config.Methods["findOne"] {
		writeError(w, http.StatusMethodNotAllowed, "findOne method not enabled for this endpoint")
		return
	}

	// ── User CT ──────────────────────────────────────────────────────────
	if info.IsSystem && slug == "users" {
		user, err := h.getUser(r.Context(), id)
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": user})
		return
	}

	// ── Regular CT ───────────────────────────────────────────────────────
	var data []byte
	var publishedAt *string
	var createdAt, updatedAt string
	err = h.db.QueryRowContext(r.Context(), `
		SELECT data, published_at::text, created_at::text, updated_at::text
		FROM entries
		WHERE id=$1 AND content_type_id=$2
		  AND published_at IS NOT NULL AND published_at <= NOW()`,
		id, info.ID,
	).Scan(&data, &publishedAt, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": marshalEntry(id, info.ID, data, publishedAt, createdAt, updatedAt),
	})
}

// POST /v1/{slug}  — create a new entry (non-system CTs only)
func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	info, err := h.loadCTInfo(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info == nil {
		writeError(w, http.StatusNotFound, "content type not found")
		return
	}
	if !h.requireAuth(w, r, info.Config) {
		return
	}
	if !info.Config.Methods["create"] {
		writeError(w, http.StatusMethodNotAllowed, "create method not enabled for this endpoint")
		return
	}
	if info.IsSystem {
		writeError(w, http.StatusForbidden,
			"cannot create entries for system content types via this endpoint")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dataJSON, _ := json.Marshal(body)

	var id int64
	var createdAt, updatedAt string
	if err := h.db.QueryRowContext(r.Context(), `
		INSERT INTO entries (content_type_id, data, published_at)
		VALUES ($1, $2, NOW())
		RETURNING id, created_at::text, updated_at::text`,
		info.ID, dataJSON,
	).Scan(&id, &createdAt, &updatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := map[string]any{"id": id, "content_type_id": info.ID,
		"created_at": createdAt, "updated_at": updatedAt}
	for k, v := range body {
		result[k] = v
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// PUT /v1/{slug}/{id}   — full replace
// PATCH /v1/{slug}/{id} — partial update (merge) when update.partial = true
func (h *Handler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	info, err := h.loadCTInfo(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info == nil {
		writeError(w, http.StatusNotFound, "content type not found")
		return
	}
	if !h.requireAuth(w, r, info.Config) {
		return
	}
	if !info.Config.Methods["update"] {
		writeError(w, http.StatusMethodNotAllowed, "update method not enabled for this endpoint")
		return
	}
	if info.IsSystem {
		writeError(w, http.StatusForbidden,
			"cannot update system content type entries via this endpoint")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// PATCH + partial feature → merge with existing data
	features := info.Config.Features["update"]
	if r.Method == http.MethodPatch && features != nil && features["partial"] {
		var existingRaw []byte
		if err := h.db.QueryRowContext(r.Context(),
			`SELECT data FROM entries WHERE id=$1 AND content_type_id=$2`,
			id, info.ID,
		).Scan(&existingRaw); err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "entry not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var existing map[string]any
		json.Unmarshal(existingRaw, &existing) //nolint:errcheck
		for k, v := range body {
			existing[k] = v
		}
		body = existing
	}

	dataJSON, _ := json.Marshal(body)
	res, err := h.db.ExecContext(r.Context(), `
		UPDATE entries SET data=$1, updated_at=NOW()
		WHERE id=$2 AND content_type_id=$3`,
		dataJSON, id, info.ID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}

	result := map[string]any{"id": id, "content_type_id": info.ID}
	for k, v := range body {
		result[k] = v
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DELETE /v1/{slug}/{id}
func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	info, err := h.loadCTInfo(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info == nil {
		writeError(w, http.StatusNotFound, "content type not found")
		return
	}
	if !h.requireAuth(w, r, info.Config) {
		return
	}
	if !info.Config.Methods["delete"] {
		writeError(w, http.StatusMethodNotAllowed, "delete method not enabled for this endpoint")
		return
	}
	if info.IsSystem {
		writeError(w, http.StatusForbidden,
			"cannot delete system content type entries via this endpoint")
		return
	}

	res, err := h.db.ExecContext(r.Context(), `
		DELETE FROM entries WHERE id=$1 AND content_type_id=$2`,
		id, info.ID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "entry not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/media?page=1&limit=50
func (h *Handler) ListMedia(w http.ResponseWriter, r *http.Request) {
	// Media is always private (requires API key)
	if !h.validateAPIKey(r) {
		writeError(w, http.StatusUnauthorized,
			"authentication required — provide a valid X-API-Key header")
		return
	}

	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 50)
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	var total int
	h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media`).Scan(&total) //nolint:errcheck

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, filename, original_name, mime_type, size, url, created_at::text
		FROM media ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var filename, originalName, mimeType, url, createdAt string
		var size int64
		rows.Scan(&id, &filename, &originalName, &mimeType, &size, &url, &createdAt) //nolint:errcheck
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
