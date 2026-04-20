package presence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eisen/teamchat/internal/shared/models"
	"github.com/redis/go-redis/v9"
)

type Manager struct {
	logger *slog.Logger
	redis  *redis.Client

	mu          sync.RWMutex
	byWorkspace map[string]map[string]models.Presence
}

func NewManager(logger *slog.Logger, redisClient *redis.Client) *Manager {
	return &Manager{
		logger:      logger,
		redis:       redisClient,
		byWorkspace: make(map[string]map[string]models.Presence),
	}
}

func (m *Manager) SetOnline(ctx context.Context, workspaceID, channel string, user models.User) models.Presence {
	m.mu.Lock()
	defer m.mu.Unlock()

	p := models.Presence{
		WorkspaceID: workspaceID,
		UserID:      user.ID,
		Handle:      user.Handle,
		Channel:     channel,
		Online:      true,
		LastSeenAt:  time.Now().UTC(),
	}
	if _, ok := m.byWorkspace[workspaceID]; !ok {
		m.byWorkspace[workspaceID] = make(map[string]models.Presence)
	}
	m.byWorkspace[workspaceID][user.ID] = p
	m.publish(ctx, p)
	return p
}

func (m *Manager) SetOffline(ctx context.Context, workspaceID, userID string) (models.Presence, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	users, ok := m.byWorkspace[workspaceID]
	if !ok {
		return models.Presence{}, false
	}
	p, ok := users[userID]
	if !ok {
		return models.Presence{}, false
	}
	p.Online = false
	p.LastSeenAt = time.Now().UTC()
	users[userID] = p
	m.publish(ctx, p)
	return p, true
}

func (m *Manager) UpdateChannel(ctx context.Context, workspaceID, userID, channel string) (models.Presence, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	users, ok := m.byWorkspace[workspaceID]
	if !ok {
		return models.Presence{}, false
	}
	p, ok := users[userID]
	if !ok {
		return models.Presence{}, false
	}
	p.Channel = channel
	p.LastSeenAt = time.Now().UTC()
	users[userID] = p
	m.publish(ctx, p)
	return p, true
}

func (m *Manager) ChangeHandle(ctx context.Context, workspaceID, userID, handle string) (models.Presence, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	users, ok := m.byWorkspace[workspaceID]
	if !ok {
		return models.Presence{}, false
	}
	p, ok := users[userID]
	if !ok {
		return models.Presence{}, false
	}
	p.Handle = handle
	p.LastSeenAt = time.Now().UTC()
	users[userID] = p
	m.publish(ctx, p)
	return p, true
}

func (m *Manager) Snapshot(workspaceID string) []models.Presence {
	m.mu.RLock()
	defer m.mu.RUnlock()

	users := m.byWorkspace[workspaceID]
	out := make([]models.Presence, 0, len(users))
	for _, p := range users {
		out = append(out, p)
	}
	return out
}

func (m *Manager) publish(ctx context.Context, presence models.Presence) {
	if m.redis == nil {
		return
	}
	raw, err := json.Marshal(presence)
	if err != nil {
		m.logger.Warn("marshal presence", "error", err)
		return
	}
	key := fmt.Sprintf("presence:%s:%s", presence.WorkspaceID, presence.UserID)
	if err := m.redis.Set(ctx, key, raw, 24*time.Hour).Err(); err != nil {
		m.logger.Warn("cache presence", "error", err)
	}
	if err := m.redis.Publish(ctx, "presence.events", raw).Err(); err != nil {
		m.logger.Warn("publish presence", "error", err)
	}
}
