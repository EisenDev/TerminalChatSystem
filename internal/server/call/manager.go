package call

import (
	"context"
	"log/slog"

	"github.com/eisen/teamchat/internal/shared/protocol"
)

type Manager interface {
	HandleSignal(ctx context.Context, userID string, event protocol.Envelope) (*protocol.Envelope, error)
}

type NoopManager struct {
	logger *slog.Logger
}

func NewNoopManager(logger *slog.Logger) *NoopManager {
	return &NoopManager{logger: logger}
}

func (m *NoopManager) HandleSignal(_ context.Context, userID string, event protocol.Envelope) (*protocol.Envelope, error) {
	m.logger.Info("call scaffold invoked", "user_id", userID, "event_type", event.Type)
	resp := protocol.MustEnvelope(protocol.ServerCallStateUpdate, protocol.CallStatePayload{
		Target: userID,
		Status: "idle",
		Note:   "voice calling is scaffolded only; integrate Pion WebRTC signaling/media here later",
	})
	return &resp, nil
}
