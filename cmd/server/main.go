package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"mutane/internal/api"
	"mutane/internal/database"
	"mutane/internal/devreload"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	handler := api.NewHandler(db)
	appRouter := api.NewRouter(handler)

	// ── Dev hot-reload ────────────────────────────────────────────────────────
	// Enabled only when APP_ENV=development.
	// Wraps the main router with a thin mux that adds GET /dev/reload (SSE).
	// Also starts a file-system poller on web/static/ so the browser reloads
	// automatically when HTML or CSS files change.
	// In production this block is never executed — no overhead whatsoever.
	var finalHandler http.Handler = appRouter

	if os.Getenv("APP_ENV") == "development" {
		devreload.Watch("web/static")

		devMux := http.NewServeMux()
		devMux.HandleFunc("GET /dev/reload", devreload.Handler)
		devMux.Handle("/", appRouter)
		finalHandler = devMux

		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Println("  🔥 DEV MODE — hot-reload active")
		log.Println("     • Go files  : air recompiles on save")
		log.Println("     • web/static: browser reloads on save")
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, finalHandler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
