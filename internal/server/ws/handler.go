package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/eisen/teamchat/internal/server/chat"
	"github.com/eisen/teamchat/internal/shared/config"
	"github.com/eisen/teamchat/internal/shared/protocol"
	"github.com/gorilla/websocket"
)

type Handler struct {
	logger   *slog.Logger
	hub      *chat.Hub
	config   config.Server
	upgrader websocket.Upgrader
}

func NewHandler(logger *slog.Logger, hub *chat.Hub, cfg config.Server) *Handler {
	return &Handler{
		logger: logger,
		hub:    hub,
		config: cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return cfg.AllowedOrigin == "*" || r.Header.Get("Origin") == cfg.AllowedOrigin
			},
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Warn("upgrade websocket", "error", err)
		return
	}

	sess := chat.NewSession(newSessionID(), remoteIP(r.RemoteAddr))
	h.hub.Register(sess)

	ctx, cancel := context.WithCancel(r.Context())
	go h.writePump(ctx, conn, sess)
	h.readPump(ctx, cancel, conn, sess)
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

func newSessionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "session"
	}
	return hex.EncodeToString(buf)
}

func (h *Handler) readPump(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, sess *chat.Session) {
	defer func() {
		cancel()
		h.hub.Unregister(sess)
		_ = conn.Close()
	}()

	conn.SetReadLimit(h.config.ReadLimitBytes)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var env protocol.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			h.logger.Info("websocket read ended", "error", err)
			return
		}
		h.hub.HandleInbound(sess, env)
	}
}

func (h *Handler) writePump(ctx context.Context, conn *websocket.Conn, sess *chat.Session) {
	ticker := time.NewTicker(h.config.PingInterval)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sess.Outbound():
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))
			raw, err := json.Marshal(msg)
			if err != nil {
				h.logger.Warn("marshal outbound message", "error", err)
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
				h.logger.Info("websocket write ended", "error", err)
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(h.config.WriteTimeout)); err != nil {
				h.logger.Info("websocket ping ended", "error", err)
				return
			}
		}
	}
}
