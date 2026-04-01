package api

import (
	"database/sql"

	"mutane/internal/admin"
	"mutane/internal/apikey"
	"mutane/internal/auth"
	"mutane/internal/content"
	"mutane/internal/media"
	"mutane/internal/public"
)

type Handler struct {
	db      *sql.DB
	Auth    *auth.Handler
	Content *content.Handler
	Media   *media.Handler
	APIKey  *apikey.Handler
	Admin   *admin.Handler
	Public  *public.Handler
	KeyRepo *apikey.Repository
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{
		db:      db,
		Auth:    auth.NewHandler(db),
		Content: content.NewHandler(db),
		Media:   media.NewHandler(db),
		APIKey:  apikey.NewHandler(db),
		Admin:   admin.NewHandler(db),
		Public:  public.NewHandler(db),
		KeyRepo: apikey.NewRepository(db),
	}
}
