package media

import (
	"context"
	"database/sql"
	"time"
)

type Media struct {
	ID           int64     `json:"id"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	Size         int64     `json:"size"`
	URL          string    `json:"url"`
	UploadedBy   *int64    `json:"uploaded_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, m *Media) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO media (filename, original_name, mime_type, size, url, uploaded_by)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		m.Filename, m.OriginalName, m.MimeType, m.Size, m.URL, m.UploadedBy,
	).Scan(&m.ID, &m.CreatedAt)
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]Media, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, filename, original_name, mime_type, size, url, uploaded_by, created_at
		 FROM media ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.ID, &m.Filename, &m.OriginalName, &m.MimeType, &m.Size, &m.URL, &m.UploadedBy, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, m)
	}
	return items, total, rows.Err()
}

func (r *Repository) GetByFilename(ctx context.Context, filename string) (*Media, error) {
	m := &Media{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, filename, original_name, mime_type, size, url, uploaded_by, created_at FROM media WHERE filename = $1`,
		filename,
	).Scan(&m.ID, &m.Filename, &m.OriginalName, &m.MimeType, &m.Size, &m.URL, &m.UploadedBy, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *Repository) Delete(ctx context.Context, id int64) (string, error) {
	var filename string
	err := r.db.QueryRowContext(ctx,
		`DELETE FROM media WHERE id = $1 RETURNING filename`, id,
	).Scan(&filename)
	return filename, err
}
