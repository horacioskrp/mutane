package api

import "net/http"

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	// ── Static assets ────────────────────────────────────────────────────────
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// ── Public pages ─────────────────────────────────────────────────────────
	mux.HandleFunc("GET /login", h.Auth.LoginPage)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// ── Auth API (public) ─────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/auth/register", h.Auth.Register)
	mux.HandleFunc("POST /api/auth/login", h.Auth.Login)
	mux.HandleFunc("POST /api/auth/logout", h.Auth.Logout)

	// ── Auth API (protected by Bearer token) ─────────────────────────────────
	mux.Handle("POST /api/auth/2fa/enable", BearerAuth(http.HandlerFunc(h.Auth.Enable2FA)))
	mux.Handle("POST /api/auth/2fa/verify", BearerAuth(http.HandlerFunc(h.Auth.Verify2FA)))

	// ── Admin JSON API (Bearer token) ─────────────────────────────────────────
	mux.Handle("GET /api/me", BearerAuth(http.HandlerFunc(h.Admin.Me)))
	mux.Handle("GET /api/stats", BearerAuth(http.HandlerFunc(h.Admin.Stats)))

	// Content types
	mux.Handle("GET /api/content-types", BearerAuth(http.HandlerFunc(h.Content.ListContentTypes)))
	mux.Handle("POST /api/content-types", BearerAuth(http.HandlerFunc(h.Content.CreateContentType)))
	mux.Handle("GET /api/content-types/{id}", BearerAuth(http.HandlerFunc(h.Content.GetContentType)))
	mux.Handle("PUT /api/content-types/{id}", BearerAuth(http.HandlerFunc(h.Content.UpdateContentType)))
	mux.Handle("DELETE /api/content-types/{id}", BearerAuth(http.HandlerFunc(h.Content.DeleteContentType)))

	// Entries
	mux.Handle("GET /api/content-types/{typeId}/entries", BearerAuth(http.HandlerFunc(h.Content.ListEntries)))
	mux.Handle("POST /api/content-types/{typeId}/entries", BearerAuth(http.HandlerFunc(h.Content.CreateEntry)))
	mux.Handle("GET /api/content-types/{typeId}/entries/{id}", BearerAuth(http.HandlerFunc(h.Content.GetEntry)))
	mux.Handle("PUT /api/content-types/{typeId}/entries/{id}", BearerAuth(http.HandlerFunc(h.Content.UpdateEntry)))
	mux.Handle("DELETE /api/content-types/{typeId}/entries/{id}", BearerAuth(http.HandlerFunc(h.Content.DeleteEntry)))

	// Media
	mux.Handle("GET /api/media", BearerAuth(http.HandlerFunc(h.Media.List)))
	mux.Handle("POST /api/media/upload", BearerAuth(http.HandlerFunc(h.Media.Upload)))
	mux.Handle("DELETE /api/media/{id}", BearerAuth(http.HandlerFunc(h.Media.Delete)))

	// API Keys
	mux.Handle("GET /api/keys", BearerAuth(http.HandlerFunc(h.APIKey.List)))
	mux.Handle("POST /api/keys", BearerAuth(http.HandlerFunc(h.APIKey.Create)))
	mux.Handle("DELETE /api/keys/{id}", BearerAuth(http.HandlerFunc(h.APIKey.Revoke)))

	// ── Admin UI (session cookie, HTMX + Templ pages) ─────────────────────────
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /admin/", h.Content.DashboardPage)
	adminMux.HandleFunc("GET /admin/content-types", h.Content.ListPage)
	adminMux.HandleFunc("GET /admin/content-types/new", h.Content.NewContentTypePage)
	adminMux.HandleFunc("GET /admin/content-types/{id}", h.Content.BuilderPage)
	adminMux.HandleFunc("GET /admin/content-types/{id}/entries", h.Content.EntryListPage)
	adminMux.HandleFunc("GET /admin/content-types/{id}/entries/new", h.Content.EntryEditorPage)
	adminMux.HandleFunc("GET /admin/content-types/{id}/entries/{eid}/edit", h.Content.EntryEditorPage)
	adminMux.HandleFunc("GET /admin/media", h.Media.ManagerPage)
	adminMux.HandleFunc("GET /admin/api-keys", h.APIKey.KeysPage)
	adminMux.HandleFunc("GET /admin/settings", h.Admin.SettingsPage)

	// HTMX action routes (within admin)
	adminMux.HandleFunc("POST /admin/content-types", h.Content.CreateContentType)
	adminMux.HandleFunc("DELETE /admin/content-types/{id}", h.Content.DeleteContentType)
	adminMux.HandleFunc("POST /admin/content-types/{id}/fields", h.Content.AddField)
	adminMux.HandleFunc("DELETE /admin/content-types/{id}/fields/{fid}", h.Content.DeleteField)
	adminMux.HandleFunc("POST /admin/content-types/{id}/entries", h.Content.CreateEntry)
	adminMux.HandleFunc("PUT /admin/content-types/{id}/entries/{eid}", h.Content.UpdateEntry)
	adminMux.HandleFunc("DELETE /admin/content-types/{id}/entries/{eid}", h.Content.DeleteEntry)
	adminMux.HandleFunc("POST /admin/media/upload", h.Media.Upload)
	adminMux.HandleFunc("DELETE /admin/media/{id}", h.Media.Delete)
	adminMux.HandleFunc("POST /admin/api-keys", h.APIKey.Create)
	adminMux.HandleFunc("DELETE /admin/api-keys/{id}", h.APIKey.Revoke)

	mux.Handle("/admin/", SessionAuth(adminMux))

	// ── Public API v1 (X-API-Key) ─────────────────────────────────────────────
	apiKeyMiddleware := APIKeyAuth(h.KeyRepo)
	mux.Handle("GET /v1/media", apiKeyMiddleware(http.HandlerFunc(h.Public.ListMedia)))
	mux.Handle("GET /v1/{slug}", apiKeyMiddleware(http.HandlerFunc(h.Public.ListEntries)))
	mux.Handle("GET /v1/{slug}/{id}", apiKeyMiddleware(http.HandlerFunc(h.Public.GetEntry)))

	return Logger(CORS(mux))
}
