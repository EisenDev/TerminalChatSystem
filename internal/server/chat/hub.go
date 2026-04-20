package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/eisen/teamchat/internal/server/auth"
	"github.com/eisen/teamchat/internal/server/call"
	"github.com/eisen/teamchat/internal/server/presence"
	"github.com/eisen/teamchat/internal/server/store"
	"github.com/eisen/teamchat/internal/shared/models"
	"github.com/eisen/teamchat/internal/shared/protocol"
)

type Session struct {
	id           string
	user         models.User
	workspace    models.Workspace
	current      models.Channel
	send         chan protocol.Envelope
	connectedAt  time.Time
	lastActivity time.Time
	sendMu       sync.RWMutex
	closed       bool
	closeOnce    sync.Once
}

type Hub struct {
	logger        *slog.Logger
	store         store.Store
	presence      *presence.Manager
	callManager   call.Manager
	defaultRoom   string
	historyLimit  int
	register      chan *Session
	unregister    chan *Session
	inbound       chan inboundEvent
	shutdown      chan struct{}

	mu                 sync.RWMutex
	sessions           map[*Session]struct{}
	sessionsByWorkspace map[string]map[*Session]struct{}
	lastPingByUser     map[string]time.Time
}

type inboundEvent struct {
	session *Session
	event   protocol.Envelope
}

func NewHub(logger *slog.Logger, st store.Store, presenceSvc *presence.Manager, callMgr call.Manager, defaultRoom string, historyLimit int) *Hub {
	return &Hub{
		logger:              logger,
		store:               st,
		presence:            presenceSvc,
		callManager:         callMgr,
		defaultRoom:         defaultRoom,
		historyLimit:        historyLimit,
		register:            make(chan *Session),
		unregister:          make(chan *Session),
		inbound:             make(chan inboundEvent, 128),
		shutdown:            make(chan struct{}),
		sessions:            make(map[*Session]struct{}),
		sessionsByWorkspace: make(map[string]map[*Session]struct{}),
		lastPingByUser:      make(map[string]time.Time),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.shutdown:
			return
		case sess := <-h.register:
			h.addSession(sess)
		case sess := <-h.unregister:
			h.removeSession(ctx, sess)
		case evt := <-h.inbound:
			if err := h.handleEvent(ctx, evt.session, evt.event); err != nil {
				h.sendError(evt.session, err)
			}
		}
	}
}

func (h *Hub) Shutdown() {
	close(h.shutdown)
}

func (h *Hub) Register(sess *Session) {
	h.register <- sess
}

func (h *Hub) Unregister(sess *Session) {
	h.unregister <- sess
}

func (h *Hub) HandleInbound(sess *Session, event protocol.Envelope) {
	h.inbound <- inboundEvent{session: sess, event: event}
}

func NewSession(id string) *Session {
	return &Session{
		id:          id,
		send:        make(chan protocol.Envelope, 64),
		connectedAt: time.Now().UTC(),
	}
}

func (h *Hub) addSession(sess *Session) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[sess] = struct{}{}
}

func (h *Hub) removeSession(ctx context.Context, sess *Session) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.sessions, sess)
	if sess.workspace.ID != "" {
		if scoped := h.sessionsByWorkspace[sess.workspace.ID]; scoped != nil {
			delete(scoped, sess)
			if len(scoped) == 0 {
				delete(h.sessionsByWorkspace, sess.workspace.ID)
			}
		}
		if presenceUpdate, ok := h.presence.SetOffline(ctx, sess.workspace.ID, sess.user.ID); ok {
			go h.broadcastPresence(sess.workspace.ID, []models.Presence{presenceUpdate})
			go h.broadcastSystem(sess.workspace.ID, protocol.ServerUserLeft, protocol.UserEventPayload{Handle: sess.user.Handle})
		}
	}
	sess.Close()
}

func (h *Hub) handleEvent(ctx context.Context, sess *Session, event protocol.Envelope) error {
	sess.lastActivity = time.Now().UTC()
	switch event.Type {
	case protocol.ClientIdentify:
		payload, err := protocol.DecodePayload[protocol.IdentifyPayload](event)
		if err != nil {
			return err
		}
		handle, err := auth.NormalizeHandle(payload.Handle)
		if err != nil {
			return err
		}
		user, err := h.store.EnsureUser(ctx, handle)
		if err != nil {
			return fmt.Errorf("ensure user: %w", err)
		}
		sess.user = user
		sess.Deliver(protocol.MustEnvelope(protocol.ServerIdentified, protocol.IdentifiedPayload{User: user}))
		return nil

	case protocol.ClientJoinWorkspace:
		if sess.user.ID == "" {
			return errors.New("identify first")
		}
		payload, err := protocol.DecodePayload[protocol.JoinWorkspacePayload](event)
		if err != nil {
			return err
		}
		name := strings.TrimSpace(payload.Workspace)
		if name == "" {
			return errors.New("workspace is required")
		}
		code := strings.TrimSpace(payload.Code)
		if code == "" {
			return errors.New("workspace code is required")
		}
		workspace, err := h.store.EnsureWorkspace(ctx, name, code, sess.user.Handle)
		if err != nil {
			return fmt.Errorf("ensure workspace: %w", err)
		}
		if err := h.store.AddWorkspaceMember(ctx, workspace.ID, sess.user.ID); err != nil {
			return fmt.Errorf("add workspace member: %w", err)
		}
		channel, err := h.store.EnsureChannel(ctx, workspace.ID, h.defaultRoom, models.ChannelKindPublic)
		if err != nil {
			return fmt.Errorf("ensure default channel: %w", err)
		}
		if err := h.store.AddChannelMember(ctx, channel.ID, sess.user.ID); err != nil {
			return fmt.Errorf("add default channel member: %w", err)
		}

		sess.workspace = workspace
		sess.current = channel
		h.attachWorkspaceSession(sess)

		channels, err := h.store.ListChannels(ctx, workspace.ID)
		if err != nil {
			return err
		}
		users, err := h.store.ListWorkspaceUsers(ctx, workspace.ID)
		if err != nil {
			return err
		}
		pres := h.presence.SetOnline(ctx, workspace.ID, channel.Name, sess.user)
		presences := h.presence.Snapshot(workspace.ID)

		sess.Deliver(protocol.MustEnvelope(protocol.ServerWorkspaceJoined, protocol.WorkspaceJoinedPayload{
			Workspace:      workspace,
			Channels:       channels,
			Users:          users,
			CurrentChannel: channel.Name,
		}))
		sess.Deliver(protocol.MustEnvelope(protocol.ServerUsersList, protocol.UsersListPayload{Users: users}))
		sess.Deliver(protocol.MustEnvelope(protocol.ServerChannelsList, protocol.ChannelsListPayload{Channels: channels}))
		sess.Deliver(protocol.MustEnvelope(protocol.ServerChannelJoined, protocol.ChannelJoinedPayload{Channel: channel}))
		h.sendHistory(ctx, sess, channel.ID, channel.Name)
		h.broadcastPresence(workspace.ID, presences)
		h.broadcastSystem(workspace.ID, protocol.ServerUserJoined, protocol.UserEventPayload{Handle: sess.user.Handle, Channel: channel.Name})
		_ = pres
		return nil

	case protocol.ClientJoinChannel:
		if sess.workspace.ID == "" {
			return errors.New("join a workspace first")
		}
		payload, err := protocol.DecodePayload[protocol.JoinChannelPayload](event)
		if err != nil {
			return err
		}
		if strings.TrimSpace(payload.Channel) == "" {
			return errors.New("channel is required")
		}
		channel, err := h.store.EnsureChannel(ctx, sess.workspace.ID, payload.Channel, models.ChannelKindPublic)
		if err != nil {
			return err
		}
		if err := h.store.AddChannelMember(ctx, channel.ID, sess.user.ID); err != nil {
			return err
		}
		sess.current = channel
		if pres, ok := h.presence.UpdateChannel(ctx, sess.workspace.ID, sess.user.ID, channel.Name); ok {
			h.broadcastPresence(sess.workspace.ID, []models.Presence{pres})
		}
		sess.Deliver(protocol.MustEnvelope(protocol.ServerChannelJoined, protocol.ChannelJoinedPayload{Channel: channel}))
		h.sendHistory(ctx, sess, channel.ID, channel.Name)
		return nil

	case protocol.ClientSendMessage:
		if sess.current.ID == "" {
			return errors.New("join a channel first")
		}
		payload, err := protocol.DecodePayload[protocol.SendMessagePayload](event)
		if err != nil {
			return err
		}
		body := strings.TrimSpace(payload.Body)
		if body == "" {
			return errors.New("message body is required")
		}
		msg, err := h.store.SaveMessage(ctx, sess.workspace.ID, sess.current.ID, sess.user, body, models.MessageTypeChat)
		if err != nil {
			return fmt.Errorf("save message: %w", err)
		}
		h.broadcastToWorkspace(sess.workspace.ID, protocol.MustEnvelope(protocol.ServerMessageNew, msg))
		return nil

	case protocol.ClientSendEmote:
		if sess.current.ID == "" {
			return errors.New("join a channel first")
		}
		payload, err := protocol.DecodePayload[protocol.SendEmotePayload](event)
		if err != nil {
			return err
		}
		if strings.TrimSpace(payload.EmoteID) == "" {
			return errors.New("emote id is required")
		}
		msg, err := h.store.SaveMessage(ctx, sess.workspace.ID, sess.current.ID, sess.user, payload.EmoteID, models.MessageTypeEmote)
		if err != nil {
			return fmt.Errorf("save emote: %w", err)
		}
		h.broadcastToWorkspace(sess.workspace.ID, protocol.MustEnvelope(protocol.ServerEmoteNew, msg))
		return nil

	case protocol.ClientRequestHistory:
		if sess.current.ID == "" {
			return errors.New("join a channel first")
		}
		h.sendHistory(ctx, sess, sess.current.ID, sess.current.Name)
		return nil

	case protocol.ClientRequestUsers:
		if sess.workspace.ID == "" {
			return errors.New("join a workspace first")
		}
		users, err := h.store.ListWorkspaceUsers(ctx, sess.workspace.ID)
		if err != nil {
			return err
		}
		sess.Deliver(protocol.MustEnvelope(protocol.ServerUsersList, protocol.UsersListPayload{Users: users}))
		sess.Deliver(protocol.MustEnvelope(protocol.ServerPresenceUpdate, protocol.PresenceUpdatePayload{
			Presences: h.presence.Snapshot(sess.workspace.ID),
		}))
		return nil

	case protocol.ClientRequestChannels:
		if sess.workspace.ID == "" {
			return errors.New("join a workspace first")
		}
		channels, err := h.store.ListChannels(ctx, sess.workspace.ID)
		if err != nil {
			return err
		}
		sess.Deliver(protocol.MustEnvelope(protocol.ServerChannelsList, protocol.ChannelsListPayload{Channels: channels}))
		return nil

	case protocol.ClientTypingStart, protocol.ClientTypingStop:
		active := event.Type == protocol.ClientTypingStart
		h.broadcastToWorkspace(sess.workspace.ID, protocol.MustEnvelope(protocol.ServerTypingUpdate, protocol.TypingUpdatePayload{
			Channel: sess.current.Name,
			Handle:  sess.user.Handle,
			Active:  active,
		}))
		return nil

	case protocol.ClientPing:
		sess.Deliver(protocol.MustEnvelope(protocol.ServerPong, protocol.PongPayload{Message: "ok"}))
		return nil

	case protocol.ClientPingUser:
		if sess.workspace.ID == "" {
			return errors.New("join a workspace first")
		}
		payload, err := protocol.DecodePayload[protocol.PingPayload](event)
		if err != nil {
			return err
		}
		target, err := auth.NormalizeHandle(payload.Handle)
		if err != nil {
			return err
		}
		effect := normalizePingEffect(payload.Effect)
		if target == sess.user.Handle {
			return errors.New("you cannot ping yourself")
		}
		if err := h.checkPingCooldown(sess.user.ID, "user", effect); err != nil {
			return err
		}
		received := protocol.PingReceivedPayload{
			From:    sess.user.Handle,
			Target:  target,
			Channel: sess.current.Name,
			Effect:  effect,
			Scope:   "user",
		}
		delivered := h.deliverToHandle(sess.workspace.ID, target, protocol.MustEnvelope(protocol.ServerPingReceived, received))
		if delivered == 0 {
			return fmt.Errorf("user %s not found", target)
		}
		_ = h.deliverToHandle(sess.workspace.ID, target, protocol.MustEnvelope(protocol.ServerPingEffect, protocol.PingEffectPayload{
			From:       sess.user.Handle,
			Target:     target,
			Channel:    sess.current.Name,
			Effect:     effect,
			Scope:      "user",
			DurationMS: pingEffectDuration(effect),
		}))
		h.deliverToHandle(sess.workspace.ID, target, protocol.MustEnvelope(protocol.ServerSystemNotice, protocol.SystemNoticePayload{
			Message: fmt.Sprintf("%s pinged you (%s)", sess.user.Handle, effect),
		}))
		sess.Deliver(protocol.MustEnvelope(protocol.ServerSystemNotice, protocol.SystemNoticePayload{
			Message: fmt.Sprintf("ping sent to %s (%s)", target, effect),
		}))
		return nil

	case protocol.ClientPingAll:
		if sess.workspace.ID == "" {
			return errors.New("join a workspace first")
		}
		payload, err := protocol.DecodePayload[protocol.PingPayload](event)
		if err != nil {
			return err
		}
		effect := normalizePingEffect(payload.Effect)
		if err := h.checkPingCooldown(sess.user.ID, "all", effect); err != nil {
			return err
		}
		envEffect := protocol.MustEnvelope(protocol.ServerPingEffect, protocol.PingEffectPayload{
			From:       sess.user.Handle,
			Target:     "all",
			Channel:    sess.current.Name,
			Effect:     effect,
			Scope:      "all",
			DurationMS: pingEffectDuration(effect),
		})
		envReceived := protocol.MustEnvelope(protocol.ServerPingReceived, protocol.PingReceivedPayload{
			From:    sess.user.Handle,
			Target:  "all",
			Channel: sess.current.Name,
			Effect:  effect,
			Scope:   "all",
		})
		h.broadcastToWorkspaceExcept(sess.workspace.ID, sess, envReceived)
		h.broadcastToWorkspaceExcept(sess.workspace.ID, sess, envEffect)
		sess.Deliver(protocol.MustEnvelope(protocol.ServerSystemNotice, protocol.SystemNoticePayload{
			Message: fmt.Sprintf("broadcast ping sent (%s)", effect),
		}))
		return nil

	case protocol.ClientChangeHandle:
		if sess.user.ID == "" {
			return errors.New("identify first")
		}
		payload, err := protocol.DecodePayload[protocol.ChangeHandlePayload](event)
		if err != nil {
			return err
		}
		newHandle, err := auth.NormalizeHandle(payload.Handle)
		if err != nil {
			return err
		}
		if newHandle == sess.user.Handle {
			return errors.New("already using that handle")
		}
		if h.handleInUse(sess.workspace.ID, newHandle, sess) {
			return fmt.Errorf("handle %s is already in use", newHandle)
		}
		oldHandle := sess.user.Handle
		user, err := h.store.UpdateUserHandle(ctx, sess.user.ID, newHandle)
		if err != nil {
			return fmt.Errorf("update user handle: %w", err)
		}
		sess.user = user
		if sess.workspace.ID != "" {
			if pres, ok := h.presence.ChangeHandle(ctx, sess.workspace.ID, sess.user.ID, newHandle); ok {
				h.broadcastPresence(sess.workspace.ID, []models.Presence{pres})
			}
			users, err := h.store.ListWorkspaceUsers(ctx, sess.workspace.ID)
			if err == nil {
				h.broadcastToWorkspace(sess.workspace.ID, protocol.MustEnvelope(protocol.ServerUsersList, protocol.UsersListPayload{Users: users}))
			}
		}
		sess.Deliver(protocol.MustEnvelope(protocol.ServerIdentified, protocol.IdentifiedPayload{User: user}))
		h.broadcastToWorkspace(sess.workspace.ID, protocol.MustEnvelope(protocol.ServerHandleChanged, protocol.HandleChangedPayload{
			OldHandle: oldHandle,
			NewHandle: newHandle,
		}))
		h.broadcastToWorkspace(sess.workspace.ID, protocol.MustEnvelope(protocol.ServerSystemNotice, protocol.SystemNoticePayload{
			Message: fmt.Sprintf("%s is now known as %s", oldHandle, newHandle),
		}))
		return nil

	case protocol.ClientCallInvite, protocol.ClientCallAccept, protocol.ClientCallReject, protocol.ClientCallHangup, protocol.ClientMuteStateChanged:
		resp, err := h.callManager.HandleSignal(ctx, sess.user.ID, event)
		if err != nil {
			return err
		}
		if resp != nil {
			sess.Deliver(*resp)
		}
		return nil
	default:
		return fmt.Errorf("unsupported event type %q", event.Type)
	}
}

func (h *Hub) attachWorkspaceSession(sess *Session) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.sessionsByWorkspace[sess.workspace.ID]; !ok {
		h.sessionsByWorkspace[sess.workspace.ID] = make(map[*Session]struct{})
	}
	h.sessionsByWorkspace[sess.workspace.ID][sess] = struct{}{}
}

func (h *Hub) broadcastToWorkspace(workspaceID string, event protocol.Envelope) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for sess := range h.sessionsByWorkspace[workspaceID] {
		if !sess.Deliver(event) {
			h.logger.Warn("dropping event for slow session", "session_id", sess.id, "event_type", event.Type)
		}
	}
}

func (h *Hub) broadcastPresence(workspaceID string, presences []models.Presence) {
	h.broadcastToWorkspace(workspaceID, protocol.MustEnvelope(protocol.ServerPresenceUpdate, protocol.PresenceUpdatePayload{
		Presences: presences,
	}))
}

func (h *Hub) broadcastSystem(workspaceID, eventType string, payload protocol.UserEventPayload) {
	h.broadcastToWorkspace(workspaceID, protocol.MustEnvelope(eventType, payload))
}

func (h *Hub) broadcastToWorkspaceExcept(workspaceID string, excluded *Session, event protocol.Envelope) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for sess := range h.sessionsByWorkspace[workspaceID] {
		if sess == excluded {
			continue
		}
		if !sess.Deliver(event) {
			h.logger.Warn("dropping event for slow session", "session_id", sess.id, "event_type", event.Type)
		}
	}
}

func (h *Hub) deliverToHandle(workspaceID, handle string, env protocol.Envelope) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	delivered := 0
	for sess := range h.sessionsByWorkspace[workspaceID] {
		if sess.user.Handle != handle {
			continue
		}
		if sess.Deliver(env) {
			delivered++
		}
	}
	return delivered
}

func (h *Hub) handleInUse(workspaceID, handle string, current *Session) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if workspaceID == "" {
		for sess := range h.sessions {
			if sess != current && sess.user.Handle == handle {
				return true
			}
		}
		return false
	}
	for sess := range h.sessionsByWorkspace[workspaceID] {
		if sess != current && sess.user.Handle == handle {
			return true
		}
	}
	return false
}

func (h *Hub) checkPingCooldown(userID, scope, effect string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now().UTC()
	key := userID + ":" + scope + ":" + effect
	cooldown := 3 * time.Second
	if scope == "all" {
		cooldown = 8 * time.Second
	}
	if effect != "normal" {
		cooldown += 2 * time.Second
	}
	if last, ok := h.lastPingByUser[key]; ok && now.Sub(last) < cooldown {
		return fmt.Errorf("ping cooldown active for %s", (cooldown - now.Sub(last)).Round(time.Second))
	}
	h.lastPingByUser[key] = now
	return nil
}

func normalizePingEffect(effect string) string {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "", "normal":
		return "normal"
	case "flash":
		return "flash"
	case "fku":
		return "fku"
	default:
		return "normal"
	}
}

func pingEffectDuration(effect string) int {
	switch effect {
	case "flash":
		return 1600
	case "fku":
		return 1800
	default:
		return 900
	}
}

func (h *Hub) sendHistory(ctx context.Context, sess *Session, channelID, channelName string) {
	history, err := h.store.ListHistory(ctx, channelID, h.historyLimit)
	if err != nil {
		h.sendError(sess, err)
		return
	}
	sess.Deliver(protocol.MustEnvelope(protocol.ServerHistoryBatch, protocol.HistoryBatchPayload{
		Channel:  channelName,
		Messages: history,
	}))
}

func (h *Hub) sendError(sess *Session, err error) {
	h.logger.Warn("hub event error", "session_id", sess.id, "error", err)
	env := protocol.MustEnvelope(protocol.ServerError, protocol.ErrorPayload{Message: err.Error()})
	sess.Deliver(env)
}

func MarshalEnvelope(event protocol.Envelope) ([]byte, error) {
	return json.Marshal(event)
}

func (s *Session) Outbound() <-chan protocol.Envelope {
	return s.send
}

func (s *Session) Deliver(env protocol.Envelope) bool {
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closed {
		return false
	}
	select {
	case s.send <- env:
		return true
	default:
		return false
	}
}

func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.sendMu.Lock()
		s.closed = true
		close(s.send)
		s.sendMu.Unlock()
	})
}
