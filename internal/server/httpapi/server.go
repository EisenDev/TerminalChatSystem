package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eisen/teamchat/internal/server/chat"
	"github.com/eisen/teamchat/internal/server/media"
	"github.com/eisen/teamchat/internal/server/store"
	serverws "github.com/eisen/teamchat/internal/server/ws"
	"github.com/eisen/teamchat/internal/shared/config"
	"github.com/eisen/teamchat/internal/shared/models"
)

type Server struct {
	httpServer *http.Server
}

func NewServer(logger *slog.Logger, hub *chat.Hub, st store.Store, mediaStorage media.Storage, cfg config.Server) *Server {
	mux := http.NewServeMux()
	mux.Handle("/ws", serverws.NewHandler(logger, hub, cfg))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"time":   time.Now().UTC(),
		})
	})
	mux.HandleFunc("/api/media/upload", mediaUploadHandler(logger, hub, st, mediaStorage, cfg))
	mux.HandleFunc("/pub/", mediaViewHandler(logger, st, cfg))
	publicDir := filepath.Join(".", "public")
	if _, err := os.Stat(publicDir); err == nil {
		downloads := http.StripPrefix("/downloads/", http.FileServer(http.Dir(filepath.Join(publicDir, "downloads"))))
		mux.Handle("/downloads/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store, max-age=0")
			downloads.ServeHTTP(w, r)
		}))
		mux.HandleFunc("/install.sh", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store, max-age=0")
			http.ServeFile(w, r, filepath.Join(publicDir, "install.sh"))
		})
		mux.HandleFunc("/update.sh", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store, max-age=0")
			http.ServeFile(w, r, filepath.Join(publicDir, "update.sh"))
		})
		mux.HandleFunc("/install.ps1", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store, max-age=0")
			http.ServeFile(w, r, filepath.Join(publicDir, "install.ps1"))
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

func mediaUploadHandler(logger *slog.Logger, hub *chat.Hub, st store.Store, mediaStorage media.Storage, cfg config.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !mediaStorage.Configured() {
			http.Error(w, "media storage not configured", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseMultipartForm(cfg.MediaMaxBytes); err != nil {
			http.Error(w, "invalid multipart upload", http.StatusBadRequest)
			return
		}
		workspace := strings.TrimSpace(r.FormValue("workspace"))
		code := strings.TrimSpace(r.FormValue("code"))
		handle := strings.TrimSpace(r.FormValue("handle"))
		deviceToken := strings.TrimSpace(r.FormValue("device_token"))
		channelName := strings.TrimSpace(r.FormValue("channel"))
		if channelName == "" {
			channelName = cfg.DefaultChannel
		}
		if workspace == "" || code == "" || deviceToken == "" {
			http.Error(w, "workspace, code, and device token are required", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := media.ReadAllLimited(file, cfg.MediaMaxBytes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = mime.TypeByExtension(filepath.Ext(header.Filename))
		}
		kind := detectMediaKind(contentType)
		ctx := r.Context()
		joinResult, err := st.JoinWorkspace(ctx, store.JoinWorkspaceRequest{
			Name:              workspace,
			Code:              code,
			RequestedHandle:   strings.ToLower(handle),
			DeviceFingerprint: deviceFingerprint(r.RemoteAddr, deviceToken),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		channel, err := st.EnsureChannel(ctx, joinResult.Workspace.ID, channelName, models.ChannelKindPublic)
		if err != nil {
			http.Error(w, "ensure channel failed", http.StatusInternalServerError)
			return
		}
		if err := st.AddChannelMember(ctx, channel.ID, joinResult.User.ID); err != nil {
			http.Error(w, "join channel failed", http.StatusInternalServerError)
			return
		}
		count, err := st.CountMediaByKind(ctx, channel.ID, kind)
		if err != nil {
			http.Error(w, "count media failed", http.StatusInternalServerError)
			return
		}
		label := fmt.Sprintf("%s #%d", kind, count+1)
		msg, err := st.SaveMessage(ctx, joinResult.Workspace.ID, channel.ID, joinResult.User, label, models.MessageTypeMedia)
		if err != nil {
			http.Error(w, "save message failed", http.StatusInternalServerError)
			return
		}
		objectKey := fmt.Sprintf("%s/%s/%s%s", workspace, channel.Name, msg.ID, filepath.Ext(header.Filename))
		publicURL, err := mediaStorage.Put(ctx, objectKey, contentType, data)
		if err != nil {
			http.Error(w, "upload to storage failed", http.StatusInternalServerError)
			return
		}
		asset, err := st.CreateMediaAsset(ctx, models.MediaAsset{
			WorkspaceID: joinResult.Workspace.ID,
			ChannelID:   channel.ID,
			MessageID:   msg.ID,
			UserID:      joinResult.User.ID,
			UserHandle:  joinResult.User.Handle,
			Kind:        kind,
			ObjectKey:   objectKey,
			FileName:    header.Filename,
			ContentType: contentType,
			ByteSize:    int64(len(data)),
			PublicURL:   publicURL,
		})
		if err != nil {
			http.Error(w, "save media metadata failed", http.StatusInternalServerError)
			return
		}
		msg.MediaID = asset.ID
		msg.MediaKind = asset.Kind
		msg.MediaURL = strings.TrimRight(cfg.PublicBaseURL, "/") + "/pub/" + asset.ID
		hub.PublishMessage(joinResult.Workspace.ID, msg)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": msg, "media": asset})
		logger.Info("media uploaded", "workspace", workspace, "channel", channel.Name, "kind", kind, "file", header.Filename)
	}
}

func mediaViewHandler(logger *slog.Logger, st store.Store, cfg config.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/pub/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		asset, err := st.GetMediaAsset(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		src := strings.TrimRight(cfg.R2PublicBase, "/") + "/" + asset.ObjectKey
		page := mediaPageTemplate
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = page.Execute(w, map[string]any{
			"Kind": asset.Kind, "FileName": asset.FileName, "ContentType": asset.ContentType, "Src": src,
		})
		logger.Info("media viewed", "id", id, "kind", asset.Kind)
	}
}

func detectMediaKind(contentType string) models.MediaKind {
	contentType = strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return models.MediaKindImage
	case strings.HasPrefix(contentType, "video/"):
		return models.MediaKindVideo
	default:
		return models.MediaKindFile
	}
}

func deviceFingerprint(remoteAddr, deviceToken string) string {
	sum := sha256.Sum256([]byte(remoteAddr + "|" + deviceToken))
	return hex.EncodeToString(sum[:])
}

var mediaPageTemplate = template.Must(template.New("media").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.FileName}} - TermiChat</title>
<style>
body{margin:0;background:#08131b;color:#ecf7ff;font-family:JetBrains Mono,monospace;padding:24px}
.wrap{max-width:1080px;margin:0 auto}.card{border:1px solid rgba(255,255,255,.12);background:#0f1824;border-radius:20px;padding:20px}
img,video{max-width:100%;max-height:80vh;display:block;margin:0 auto}.meta{color:#9eb0bf;margin-bottom:16px}a{color:#68dcff}
</style></head><body><div class="wrap"><div class="card"><div class="meta">{{.Kind}} · {{.FileName}}</div>{{if eq .Kind "image"}}<img src="{{.Src}}" alt="{{.FileName}}">{{else if eq .Kind "video"}}<video src="{{.Src}}" controls playsinline></video>{{else}}<p><a href="{{.Src}}" target="_blank" rel="noopener">Open file</a></p>{{end}}</div></div></body></html>`))

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
