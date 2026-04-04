package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// scanEndpointConfig unmarshals raw JSONB bytes into an EndpointConfig,
// falling back to defaults for empty / missing data.
func scanEndpointConfig(raw []byte) EndpointConfig {
	cfg := DefaultEndpointConfig()
	if len(raw) > 2 {
		_ = json.Unmarshal(raw, &cfg)
	}
	if cfg.Methods == nil {
		cfg.Methods = DefaultEndpointConfig().Methods
	}
	if cfg.Features == nil {
		cfg.Features = DefaultEndpointConfig().Features
	}
	return cfg
}

// isUserCT returns true when the given content_type_id maps to the system User CT.
func (r *Repository) isUserCT(ctx context.Context, id int64) bool {
	var isSystem bool
	var slug string
	err := r.db.QueryRowContext(ctx,
		`SELECT is_system, slug FROM content_types WHERE id=$1`, id,
	).Scan(&isSystem, &slug)
	return err == nil && isSystem && slug == "users"
}

// ── ContentType CRUD ─────────────────────────────────────────────────────────

func (r *Repository) CreateContentType(ctx context.Context, ct *ContentType) error {
	cfgJSON, _ := json.Marshal(DefaultEndpointConfig())
	ct.EndpointConfig = DefaultEndpointConfig()
	return r.db.QueryRowContext(ctx,
		`INSERT INTO content_types (name, slug, description, endpoint_config)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at, updated_at`,
		ct.Name, ct.Slug, ct.Description, cfgJSON,
	).Scan(&ct.ID, &ct.CreatedAt, &ct.UpdatedAt)
}

func (r *Repository) GetContentType(ctx context.Context, id int64) (*ContentType, error) {
	ct := &ContentType{}
	var cfgRaw []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, is_system, endpoint_config, created_at, updated_at
		 FROM content_types WHERE id = $1`,
		id,
	).Scan(&ct.ID, &ct.Name, &ct.Slug, &ct.Description, &ct.IsSystem, &cfgRaw, &ct.CreatedAt, &ct.UpdatedAt)
	if err != nil {
		return nil, err
	}
	ct.EndpointConfig = scanEndpointConfig(cfgRaw)

	fields, err := r.getFields(ctx, id)
	if err != nil {
		return nil, err
	}

	// User CT: prepend built-in fields
	if ct.IsSystem && ct.Slug == "users" {
		ct.Fields = append(userBuiltinFields(ct.ID), fields...)
	} else {
		ct.Fields = fields
	}
	return ct, nil
}

func (r *Repository) ListContentTypes(ctx context.Context) ([]ContentType, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ct.id, ct.name, ct.slug, ct.description, ct.is_system, ct.endpoint_config,
		       ct.created_at, ct.updated_at,
		       f.id, f.content_type_id, f.name, f.type, f.required, f."order"
		FROM content_types ct
		LEFT JOIN fields f ON f.content_type_id = ct.id
		ORDER BY ct.is_system DESC, ct.name, f."order"
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexMap := make(map[int64]int)
	var result []ContentType

	for rows.Next() {
		var ct ContentType
		var cfgRaw []byte
		var (
			fID       sql.NullInt64
			fCTID     sql.NullInt64
			fName     sql.NullString
			fType     sql.NullString
			fRequired sql.NullBool
			fOrder    sql.NullInt32
		)
		if err := rows.Scan(
			&ct.ID, &ct.Name, &ct.Slug, &ct.Description, &ct.IsSystem, &cfgRaw,
			&ct.CreatedAt, &ct.UpdatedAt,
			&fID, &fCTID, &fName, &fType, &fRequired, &fOrder,
		); err != nil {
			return nil, err
		}

		idx, exists := indexMap[ct.ID]
		if !exists {
			ct.EndpointConfig = scanEndpointConfig(cfgRaw)
			// User CT: start with built-in fields
			if ct.IsSystem && ct.Slug == "users" {
				ct.Fields = userBuiltinFields(ct.ID)
			} else {
				ct.Fields = []Field{}
			}
			result = append(result, ct)
			idx = len(result) - 1
			indexMap[ct.ID] = idx
		}

		if fID.Valid {
			result[idx].Fields = append(result[idx].Fields, Field{
				ID:            fID.Int64,
				ContentTypeID: fCTID.Int64,
				Name:          fName.String,
				Type:          FieldType(fType.String),
				Required:      fRequired.Bool,
				Order:         int(fOrder.Int32),
			})
		}
	}
	return result, rows.Err()
}

func (r *Repository) UpdateContentType(ctx context.Context, ct *ContentType) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE content_types SET name=$1, slug=$2, description=$3, updated_at=NOW()
		 WHERE id=$4 AND is_system=FALSE`,
		ct.Name, ct.Slug, ct.Description, ct.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("content type not found or is a system type")
	}
	return nil
}

func (r *Repository) DeleteContentType(ctx context.Context, id int64) error {
	var isSystem bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT is_system FROM content_types WHERE id=$1`, id,
	).Scan(&isSystem); err != nil {
		return err
	}
	if isSystem {
		return fmt.Errorf("cannot delete system content type")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM content_types WHERE id = $1`, id)
	return err
}

// ── Endpoint configuration ────────────────────────────────────────────────────

func (r *Repository) UpdateEndpointConfig(ctx context.Context, id int64, cfg EndpointConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE content_types SET endpoint_config=$1, updated_at=NOW() WHERE id=$2`,
		raw, id,
	)
	return err
}

// ── Fields ────────────────────────────────────────────────────────────────────

func (r *Repository) AddField(ctx context.Context, f *Field) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO fields (content_type_id, name, type, required, "order")
		 VALUES ($1, $2, $3, $4,
		   COALESCE((SELECT MAX("order")+1 FROM fields WHERE content_type_id=$1), 0))
		 RETURNING id, "order"`,
		f.ContentTypeID, f.Name, f.Type, f.Required,
	).Scan(&f.ID, &f.Order)
}

func (r *Repository) DeleteField(ctx context.Context, id int64) error {
	if id < 0 {
		return fmt.Errorf("cannot delete built-in field")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM fields WHERE id = $1`, id)
	return err
}

func (r *Repository) ReorderFields(ctx context.Context, ctID int64, fieldIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, id := range fieldIDs {
		if id < 0 {
			continue // skip virtual built-in field IDs
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE fields SET "order" = $1 WHERE id = $2 AND content_type_id = $3`,
			i, id, ctID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) getFields(ctx context.Context, contentTypeID int64) ([]Field, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, content_type_id, name, type, required, "order"
		 FROM fields WHERE content_type_id = $1 ORDER BY "order"`,
		contentTypeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fields []Field
	for rows.Next() {
		var f Field
		if err := rows.Scan(&f.ID, &f.ContentTypeID, &f.Name, &f.Type, &f.Required, &f.Order); err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

// ── Entry CRUD — routes to user table for system User CT ──────────────────────

func (r *Repository) CreateEntry(ctx context.Context, e *Entry) error {
	if r.isUserCT(ctx, e.ContentTypeID) {
		return r.createUserEntry(ctx, e)
	}
	data, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	return r.db.QueryRowContext(ctx,
		`INSERT INTO entries (content_type_id, data) VALUES ($1, $2) RETURNING id, created_at, updated_at`,
		e.ContentTypeID, data,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
}

func (r *Repository) GetEntry(ctx context.Context, id int64) (*Entry, error) {
	// We can't determine CT without querying first — caller passes ID only.
	// Check entries table; if not found, the caller that knows CT must handle it.
	e := &Entry{}
	var data []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, content_type_id, data, published_at, created_at, updated_at FROM entries WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.ContentTypeID, &data, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &e.Data); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	return e, nil
}

// GetEntryForCT fetches an entry, routing to the users table when appropriate.
func (r *Repository) GetEntryForCT(ctx context.Context, ctID, entryID int64) (*Entry, error) {
	if r.isUserCT(ctx, ctID) {
		return r.getUserEntry(ctx, ctID, entryID)
	}
	e := &Entry{}
	var data []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, content_type_id, data, published_at, created_at, updated_at
		 FROM entries WHERE id=$1 AND content_type_id=$2`,
		entryID, ctID,
	).Scan(&e.ID, &e.ContentTypeID, &data, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &e.Data); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	return e, nil
}

func (r *Repository) ListEntries(ctx context.Context, contentTypeID int64) ([]Entry, error) {
	if r.isUserCT(ctx, contentTypeID) {
		return r.listUserEntries(ctx, contentTypeID)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, content_type_id, data, published_at, created_at, updated_at
		 FROM entries WHERE content_type_id = $1 ORDER BY created_at DESC`,
		contentTypeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var data []byte
		if err := rows.Scan(&e.ID, &e.ContentTypeID, &data, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &e.Data); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *Repository) UpdateEntry(ctx context.Context, e *Entry) error {
	if r.isUserCT(ctx, e.ContentTypeID) {
		return r.updateUserEntry(ctx, e)
	}
	data, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE entries SET data=$1, updated_at=NOW() WHERE id=$2`,
		data, e.ID,
	)
	return err
}

func (r *Repository) DeleteEntry(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM entries WHERE id = $1`, id)
	return err
}

// DeleteEntryForCT deletes an entry, routing to users table when appropriate.
func (r *Repository) DeleteEntryForCT(ctx context.Context, ctID, entryID int64) error {
	if r.isUserCT(ctx, ctID) {
		_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, entryID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM entries WHERE id=$1 AND content_type_id=$2`, entryID, ctID)
	return err
}

// ── User CT entry helpers ─────────────────────────────────────────────────────

// userRowToEntry maps a users row into the standard Entry shape.
// data = { name, email, is_admin, ...users.data custom fields }
func userRowToEntry(ctID, id int64, name, email string, isAdmin bool, dataRaw []byte, createdAt, updatedAt time.Time) Entry {
	d := map[string]any{
		"name":     name,
		"email":    email,
		"is_admin": isAdmin,
	}
	var custom map[string]any
	if json.Unmarshal(dataRaw, &custom) == nil {
		for k, v := range custom {
			d[k] = v
		}
	}
	pub := createdAt // users are always "active"
	return Entry{
		ID:            id,
		ContentTypeID: ctID,
		Data:          d,
		PublishedAt:   &pub,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}
}

func (r *Repository) listUserEntries(ctx context.Context, ctID int64) ([]Entry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, email, is_admin, data, created_at, updated_at
		 FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var id int64
		var name, email string
		var isAdmin bool
		var dataRaw []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &email, &isAdmin, &dataRaw, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, userRowToEntry(ctID, id, name, email, isAdmin, dataRaw, createdAt, updatedAt))
	}
	return entries, rows.Err()
}

func (r *Repository) getUserEntry(ctx context.Context, ctID, userID int64) (*Entry, error) {
	var id int64
	var name, email string
	var isAdmin bool
	var dataRaw []byte
	var createdAt, updatedAt time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, email, is_admin, data, created_at, updated_at FROM users WHERE id=$1`, userID,
	).Scan(&id, &name, &email, &isAdmin, &dataRaw, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	e := userRowToEntry(ctID, id, name, email, isAdmin, dataRaw, createdAt, updatedAt)
	return &e, nil
}

func (r *Repository) createUserEntry(ctx context.Context, e *Entry) error {
	name, _ := e.Data["name"].(string)
	email, _ := e.Data["email"].(string)
	password, _ := e.Data["password"].(string)
	isAdmin, _ := e.Data["is_admin"].(bool)

	if name == "" || email == "" || password == "" {
		return fmt.Errorf("name, email and password are required for user creation")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Custom fields = everything except built-in keys
	custom := map[string]any{}
	builtinKeys := map[string]bool{"name": true, "email": true, "password": true, "is_admin": true}
	for k, v := range e.Data {
		if !builtinKeys[k] {
			custom[k] = v
		}
	}
	customJSON, _ := json.Marshal(custom)

	return r.db.QueryRowContext(ctx,
		`INSERT INTO users (name, email, password_hash, is_admin, data)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		name, email, string(hash), isAdmin, customJSON,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
}

func (r *Repository) updateUserEntry(ctx context.Context, e *Entry) error {
	// Only update fields present in e.Data
	name, hasName := e.Data["name"].(string)
	email, hasEmail := e.Data["email"].(string)
	password, hasPassword := e.Data["password"].(string)
	isAdmin, hasAdmin := e.Data["is_admin"].(bool)

	// Custom fields
	custom := map[string]any{}
	builtinKeys := map[string]bool{"name": true, "email": true, "password": true, "is_admin": true}
	for k, v := range e.Data {
		if !builtinKeys[k] {
			custom[k] = v
		}
	}
	customJSON, _ := json.Marshal(custom)

	// Build dynamic UPDATE
	setClauses := []string{"data=$1", "updated_at=NOW()"}
	args := []any{customJSON}

	argN := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if hasName && name != "" {
		setClauses = append(setClauses, "name="+argN(name))
	}
	if hasEmail && email != "" {
		setClauses = append(setClauses, "email="+argN(email))
	}
	if hasAdmin {
		setClauses = append(setClauses, "is_admin="+argN(isAdmin))
	}
	if hasPassword && password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		setClauses = append(setClauses, "password_hash="+argN(string(hash)))
	}

	args = append(args, e.ID)
	query := fmt.Sprintf("UPDATE users SET %s", joinClauses(setClauses))
	query += fmt.Sprintf(" WHERE id=$%d", len(args))

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func joinClauses(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += ","
		}
		s += p
	}
	return s
}
