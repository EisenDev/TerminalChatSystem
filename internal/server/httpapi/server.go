package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/eisen/teamchat/internal/server/chat"
	serverws "github.com/eisen/teamchat/internal/server/ws"
	"github.com/eisen/teamchat/internal/shared/config"
)

type Server struct {
	httpServer *http.Server
}

func NewServer(logger *slog.Logger, hub *chat.Hub, cfg config.Server) *Server {
	mux := http.NewServeMux()
	mux.Handle("/ws", serverws.NewHandler(logger, hub, cfg))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"time":   time.Now().UTC(),
		})
	})
	publicDir := filepath.Join(".", "public")
	if _, err := os.Stat(publicDir); err == nil {
		mux.Handle("/downloads/", http.StripPrefix("/downloads/", http.FileServer(http.Dir(filepath.Join(publicDir, "downloads")))))
		mux.HandleFunc("/install.sh", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(publicDir, "install.sh"))
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(publicDir, "index.html"))
		})
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
