package admin

import (
	"context"
	"database/sql"
)

type Stats struct {
	ContentTypes int `json:"content_types"`
	Entries      int `json:"entries"`
	Media        int `json:"media"`
	Users        int `json:"users"`
}

func LoadStats(ctx context.Context, db *sql.DB) (*Stats, error) {
	s := &Stats{}
	queries := []struct {
		dest  *int
		query string
	}{
		{&s.ContentTypes, `SELECT COUNT(*) FROM content_types`},
		{&s.Entries, `SELECT COUNT(*) FROM entries`},
		{&s.Media, `SELECT COUNT(*) FROM media`},
		{&s.Users, `SELECT COUNT(*) FROM users`},
	}
	for _, q := range queries {
		if err := db.QueryRowContext(ctx, q.query).Scan(q.dest); err != nil {
			return nil, err
		}
	}
	return s, nil
}
