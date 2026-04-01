package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// ContentType CRUD

func (r *Repository) CreateContentType(ctx context.Context, ct *ContentType) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO content_types (name, slug, description) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at`,
		ct.Name, ct.Slug, ct.Description,
	).Scan(&ct.ID, &ct.CreatedAt, &ct.UpdatedAt)
}

func (r *Repository) GetContentType(ctx context.Context, id int64) (*ContentType, error) {
	ct := &ContentType{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, created_at, updated_at FROM content_types WHERE id = $1`,
		id,
	).Scan(&ct.ID, &ct.Name, &ct.Slug, &ct.Description, &ct.CreatedAt, &ct.UpdatedAt)
	if err != nil {
		return nil, err
	}

	fields, err := r.getFields(ctx, id)
	if err != nil {
		return nil, err
	}
	ct.Fields = fields
	return ct, nil
}

func (r *Repository) ListContentTypes(ctx context.Context) ([]ContentType, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, slug, description, created_at, updated_at FROM content_types ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []ContentType
	for rows.Next() {
		var ct ContentType
		if err := rows.Scan(&ct.ID, &ct.Name, &ct.Slug, &ct.Description, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
			return nil, err
		}
		types = append(types, ct)
	}
	return types, rows.Err()
}

func (r *Repository) UpdateContentType(ctx context.Context, ct *ContentType) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE content_types SET name=$1, slug=$2, description=$3, updated_at=NOW() WHERE id=$4`,
		ct.Name, ct.Slug, ct.Description, ct.ID,
	)
	return err
}

func (r *Repository) DeleteContentType(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM content_types WHERE id = $1`, id)
	return err
}

func (r *Repository) AddField(ctx context.Context, f *Field) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO fields (content_type_id, name, type, required, "order") VALUES ($1, $2, $3, $4, COALESCE((SELECT MAX("order")+1 FROM fields WHERE content_type_id=$1),0)) RETURNING id, "order"`,
		f.ContentTypeID, f.Name, f.Type, f.Required,
	).Scan(&f.ID, &f.Order)
}

func (r *Repository) DeleteField(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM fields WHERE id = $1`, id)
	return err
}

func (r *Repository) getFields(ctx context.Context, contentTypeID int64) ([]Field, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, content_type_id, name, type, required, "order" FROM fields WHERE content_type_id = $1 ORDER BY "order"`,
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

// Entry CRUD

func (r *Repository) CreateEntry(ctx context.Context, e *Entry) error {
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

func (r *Repository) ListEntries(ctx context.Context, contentTypeID int64) ([]Entry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, content_type_id, data, published_at, created_at, updated_at FROM entries WHERE content_type_id = $1 ORDER BY created_at DESC`,
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
