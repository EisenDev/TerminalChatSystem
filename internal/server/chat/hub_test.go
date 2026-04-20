package chat

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/eisen/teamchat/internal/server/call"
	"github.com/eisen/teamchat/internal/server/presence"
	"github.com/eisen/teamchat/internal/server/store"
	"github.com/eisen/teamchat/internal/shared/models"
	"github.com/eisen/teamchat/internal/shared/protocol"
)

type fakeStore struct {
	user      models.User
	workspace models.Workspace
	channel   models.Channel
	messages  []models.Message
}

func (f *fakeStore) JoinWorkspace(_ context.Context, req store.JoinWorkspaceRequest) (store.JoinWorkspaceResult, error) {
	handle := req.RequestedHandle
	if handle == "" {
		handle = "alice"
	}
	f.user = models.User{ID: "u1", Handle: handle}
	f.workspace = models.Workspace{ID: "w1", Name: req.Name, OwnerHandle: handle, OwnerUserID: f.user.ID}
	return store.JoinWorkspaceResult{Workspace: f.workspace, User: f.user}, nil
}
func (f *fakeStore) EnsureChannel(_ context.Context, workspaceID, name string, kind models.ChannelKind) (models.Channel, error) {
	f.channel = models.Channel{ID: "c1", WorkspaceID: workspaceID, Name: name, Kind: kind}
	return f.channel, nil
}
func (f *fakeStore) AddWorkspaceMember(context.Context, string, string) error { return nil }
func (f *fakeStore) AddChannelMember(context.Context, string, string) error   { return nil }
func (f *fakeStore) ListChannels(context.Context, string) ([]models.Channel, error) {
	return []models.Channel{f.channel}, nil
}
func (f *fakeStore) ListWorkspaceUsers(context.Context, string) ([]models.User, error) {
	return []models.User{f.user}, nil
}
func (f *fakeStore) ListHistory(context.Context, string, int) ([]models.Message, error) {
	return nil, nil
}
func (f *fakeStore) SaveMessage(_ context.Context, workspaceID, channelID string, user models.User, body string, messageType models.MessageType) (models.Message, error) {
	msg := models.Message{
		ID:          "m1",
		WorkspaceID: workspaceID,
		ChannelID:   channelID,
		ChannelName: f.channel.Name,
		UserID:      user.ID,
		UserHandle:  user.Handle,
		Body:        body,
		MessageType: messageType,
		CreatedAt:   time.Now().UTC(),
	}
	f.messages = append(f.messages, msg)
	return msg, nil
}
func (f *fakeStore) UpdateUserHandle(_ context.Context, userID, handle string) (models.User, error) {
	f.user.Handle = handle
	f.user.ID = userID
	return f.user, nil
}

func TestHubMessageFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	st := &fakeStore{}
	hub := NewHub(logger, st, presence.NewManager(logger, nil), call.NewNoopManager(logger), "lobby", 50)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	sess := NewSession("s1", "127.0.0.1")
	hub.Register(sess)
	hub.HandleInbound(sess, protocol.MustEnvelope(protocol.ClientIdentify, protocol.IdentifyPayload{Handle: "alice", DeviceToken: "device-1"}))
	hub.HandleInbound(sess, protocol.MustEnvelope(protocol.ClientJoinWorkspace, protocol.JoinWorkspacePayload{Workspace: "acme", Code: "acme123"}))
	hub.HandleInbound(sess, protocol.MustEnvelope(protocol.ClientSendMessage, protocol.SendMessagePayload{Body: "hello"}))

	time.Sleep(50 * time.Millisecond)
	if len(st.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(st.messages))
	}
	if st.messages[0].Body != "hello" {
		t.Fatalf("expected body hello, got %q", st.messages[0].Body)
	}
}

type testWriter struct{ t *testing.T }

func (tw testWriter) Write(p []byte) (int, error) {
	tw.t.Log(string(p))
	return len(p), nil
}
