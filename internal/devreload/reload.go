// Package devreload provides a zero-dependency hot-reload system for development.
//
// It exposes two things:
//   - An SSE endpoint (GET /dev/reload) that browsers subscribe to.
//   - A file-system poller that broadcasts a reload signal whenever a watched
//     file changes.
//
// When the Go server itself restarts (via air), the SSE connection drops.
// The browser-side client detects the reconnection and triggers a full page
// reload, so both Go changes and static-file changes are covered.
//
// The package is only wired into the server when APP_ENV=development.
package devreload

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ── SSE Hub ───────────────────────────────────────────────────────────────────

type hub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

var global = &hub{clients: make(map[chan string]struct{})}

// subscribe registers a new SSE client and returns its channel + unsubscribe func.
func (h *hub) subscribe() (chan string, func()) {
	ch := make(chan string, 4)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}
}

// broadcast sends an event message to every connected client.
func (h *hub) broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default: // drop if client is slow
		}
	}
}

// ClientCount returns the number of currently connected SSE clients.
func ClientCount() int {
	global.mu.Lock()
	defer global.mu.Unlock()
	return len(global.clients)
}

// ── SSE HTTP handler ──────────────────────────────────────────────────────────

// Handler serves the SSE stream at GET /dev/reload.
// The browser reconnects automatically after a server restart; on reconnect
// the client detects it was previously connected and reloads the page.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering if proxied

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch, unsub := global.subscribe()
	defer unsub()

	// Initial heartbeat so the browser knows the connection is alive.
	fmt.Fprintf(w, "event: connected\ndata: ok\n\n")
	flusher.Flush()

	ticker := time.NewTicker(25 * time.Second) // keep-alive ping every 25s
	defer ticker.Stop()

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: 1\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			// SSE comment line — keeps the connection alive through proxies.
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ── File watcher ──────────────────────────────────────────────────────────────

// Watch starts a polling watcher on each provided directory.
// When any file's mtime changes, it broadcasts a "reload" event to all SSE clients.
// Poll interval: 350ms — fast enough to feel instant, cheap enough to not matter.
func Watch(dirs ...string) {
	snapshot := buildSnapshot(dirs)

	go func() {
		for {
			time.Sleep(350 * time.Millisecond)

			current := buildSnapshot(dirs)
			if hasChanges(snapshot, current) {
				log.Printf("[devreload] change detected — %d client(s) notified", ClientCount())
				global.broadcast("reload")
				snapshot = current
			}
		}
	}()

	log.Printf("[devreload] watching %v", dirs)
}

// buildSnapshot walks the given dirs and records each file's size + mtime.
func buildSnapshot(dirs []string) map[string][2]int64 {
	m := make(map[string][2]int64)
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			m[path] = [2]int64{info.Size(), info.ModTime().UnixNano()}
			return nil
		})
	}
	return m
}

// hasChanges returns true if any file was added, removed, or modified.
func hasChanges(old, new map[string][2]int64) bool {
	if len(old) != len(new) {
		return true
	}
	for path, stat := range new {
		if old[path] != stat {
			return true
		}
	}
	return false
}
