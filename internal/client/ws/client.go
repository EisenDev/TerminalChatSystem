package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eisen/teamchat/internal/shared/config"
	"github.com/eisen/teamchat/internal/shared/protocol"
	"github.com/gorilla/websocket"
)

type Status struct {
	Connected bool
	Message   string
}

type Event struct {
	Status   *Status
	Envelope *protocol.Envelope
	Err      error
}

type Session struct {
	Handle    string
	Workspace string
	Code      string
	Channel   string
}

type Manager struct {
	logger *slog.Logger
	cfg    config.Client

	mu      sync.RWMutex
	conn    *websocket.Conn
	session Session
	events  chan Event
	send    chan protocol.Envelope
	closeCh chan struct{}
	closeOnce sync.Once
}

func NewManager(logger *slog.Logger, cfg config.Client) *Manager {
	return &Manager{
		logger:  logger,
		cfg:     cfg,
		events:  make(chan Event, 128),
		send:    make(chan protocol.Envelope, 128),
		closeCh: make(chan struct{}),
	}
}

func (m *Manager) Events() <-chan Event {
	return m.events
}

func (m *Manager) Configure(session Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = session
}

func (m *Manager) Start(ctx context.Context) {
	go m.run(ctx)
}

func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.closeCh)
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.conn != nil {
			_ = m.conn.Close()
		}
	})
}

func (m *Manager) Send(eventType string, payload any) error {
	env, err := protocol.NewEnvelope(eventType, payload)
	if err != nil {
		return err
	}
	select {
	case m.send <- env:
		return nil
	default:
		return fmt.Errorf("outbound queue full")
	}
}

func (m *Manager) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.closeCh:
			return
		default:
		}

		conn, err := m.connect()
		if err != nil {
			m.events <- Event{Status: &Status{Connected: false, Message: "reconnecting"}, Err: err}
			time.Sleep(m.cfg.ReconnectDelay)
			continue
		}

		m.mu.Lock()
		m.conn = conn
		m.mu.Unlock()
		m.events <- Event{Status: &Status{Connected: true, Message: "connected"}}
		m.resync()

		readDone := make(chan struct{})
		go m.readLoop(readDone, conn)
		err = m.writeLoop(ctx, conn, readDone)
		m.events <- Event{Status: &Status{Connected: false, Message: "disconnected"}, Err: err}
		_ = conn.Close()
		time.Sleep(m.cfg.ReconnectDelay)
	}
}

func (m *Manager) connect() (*websocket.Conn, error) {
	wsURL, err := normalizeWSURL(m.cfg.ServerURL)
	if err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (m *Manager) resync() {
	m.mu.RLock()
	session := m.session
	m.mu.RUnlock()

	if session.Handle == "" || session.Workspace == "" {
		return
	}
	_ = m.Send(protocol.ClientIdentify, protocol.IdentifyPayload{Handle: session.Handle})
	_ = m.Send(protocol.ClientJoinWorkspace, protocol.JoinWorkspacePayload{Workspace: session.Workspace, Code: session.Code})
	if session.Channel != "" {
		_ = m.Send(protocol.ClientJoinChannel, protocol.JoinChannelPayload{Channel: session.Channel})
	}
}

func (m *Manager) readLoop(done chan<- struct{}, conn *websocket.Conn) {
	defer close(done)
	for {
		var env protocol.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			m.events <- Event{Err: err}
			return
		}
		m.events <- Event{Envelope: &env}
	}
}

func (m *Manager) writeLoop(ctx context.Context, conn *websocket.Conn, readDone <-chan struct{}) error {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.closeCh:
			return nil
		case <-readDone:
			return fmt.Errorf("connection closed")
		case env := <-m.send:
			raw, err := json.Marshal(env)
			if err != nil {
				return err
			}
			if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
				return err
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second)); err != nil {
				return err
			}
		}
	}
}

func normalizeWSURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported server url scheme %q", u.Scheme)
	}
	u.Path = "/ws"
	return u.String(), nil
}
