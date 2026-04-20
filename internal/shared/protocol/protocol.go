package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/eisen/teamchat/internal/shared/models"
)

const (
	ClientIdentify         = "identify"
	ClientJoinWorkspace    = "join_workspace"
	ClientJoinChannel      = "join_channel"
	ClientSendMessage      = "send_message"
	ClientSendEmote        = "send_emote"
	ClientRequestHistory   = "request_history"
	ClientPing             = "ping"
	ClientPingUser         = "ping_user"
	ClientPingAll          = "ping_all"
	ClientTypingStart      = "typing_start"
	ClientTypingStop       = "typing_stop"
	ClientRequestUsers     = "request_users"
	ClientRequestChannels  = "request_channels"
	ClientChangeHandle     = "change_handle"
	ClientCallInvite       = "call_invite"
	ClientCallAccept       = "call_accept"
	ClientCallReject       = "call_reject"
	ClientCallHangup       = "call_hangup"
	ClientMuteStateChanged = "mute_state_changed"
)

const (
	ServerIdentified        = "identified"
	ServerWorkspaceJoined   = "workspace_joined"
	ServerChannelJoined     = "channel_joined"
	ServerMessageNew        = "message_new"
	ServerEmoteNew          = "emote_new"
	ServerHistoryBatch      = "history_batch"
	ServerPresenceUpdate    = "presence_update"
	ServerUsersList         = "users_list"
	ServerChannelsList      = "channels_list"
	ServerUserJoined        = "user_joined"
	ServerUserLeft          = "user_left"
	ServerTypingUpdate      = "typing_update"
	ServerError             = "error"
	ServerPong              = "pong"
	ServerPingReceived      = "ping_received"
	ServerPingEffect        = "ping_effect"
	ServerHandleChanged     = "handle_changed"
	ServerSystemNotice      = "system_notice"
	ServerReconnectRequired = "reconnect_required"
	ServerCallInvitation    = "call_invitation"
	ServerCallStateUpdate   = "call_state_update"
)

type Envelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

func NewEnvelope(eventType string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal payload: %w", err)
	}
	return Envelope{Type: eventType, Payload: raw}, nil
}

func MustEnvelope(eventType string, payload any) Envelope {
	env, err := NewEnvelope(eventType, payload)
	if err != nil {
		panic(err)
	}
	return env
}

func DecodePayload[T any](env Envelope) (T, error) {
	var out T
	if len(env.Payload) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(env.Payload, &out); err != nil {
		return out, fmt.Errorf("decode %s payload: %w", env.Type, err)
	}
	return out, nil
}

type IdentifyPayload struct {
	Handle string `json:"handle"`
}

type JoinWorkspacePayload struct {
	Workspace string `json:"workspace"`
	Code      string `json:"code"`
}

type JoinChannelPayload struct {
	Channel string `json:"channel"`
}

type SendMessagePayload struct {
	Channel string `json:"channel"`
	Body    string `json:"body"`
}

type SendEmotePayload struct {
	Channel string `json:"channel"`
	EmoteID string `json:"emote_id"`
}

type RequestHistoryPayload struct {
	Channel string `json:"channel"`
	Limit   int    `json:"limit"`
}

type TypingPayload struct {
	Channel string `json:"channel"`
}

type PingPayload struct {
	Handle string `json:"handle,omitempty"`
	Effect string `json:"effect,omitempty"`
	Scope  string `json:"scope,omitempty"`
}

type ChangeHandlePayload struct {
	Handle string `json:"handle"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

type IdentifiedPayload struct {
	User models.User `json:"user"`
}

type WorkspaceJoinedPayload struct {
	Workspace      models.Workspace  `json:"workspace"`
	Channels       []models.Channel  `json:"channels"`
	Users          []models.User     `json:"users"`
	CurrentChannel string            `json:"current_channel"`
}

type ChannelJoinedPayload struct {
	Channel models.Channel `json:"channel"`
}

type HistoryBatchPayload struct {
	Channel  string           `json:"channel"`
	Messages []models.Message `json:"messages"`
}

type PresenceUpdatePayload struct {
	Presences []models.Presence `json:"presences"`
}

type UsersListPayload struct {
	Users []models.User `json:"users"`
}

type ChannelsListPayload struct {
	Channels []models.Channel `json:"channels"`
}

type TypingUpdatePayload struct {
	Channel string `json:"channel"`
	Handle  string `json:"handle"`
	Active  bool   `json:"active"`
}

type UserEventPayload struct {
	Handle  string `json:"handle"`
	Channel string `json:"channel,omitempty"`
}

type PingReceivedPayload struct {
	From    string `json:"from"`
	Target  string `json:"target,omitempty"`
	Channel string `json:"channel,omitempty"`
	Effect  string `json:"effect,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

type PingEffectPayload struct {
	From       string `json:"from"`
	Target     string `json:"target,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Effect     string `json:"effect"`
	Scope      string `json:"scope"`
	DurationMS int    `json:"duration_ms"`
}

type HandleChangedPayload struct {
	OldHandle string `json:"old_handle"`
	NewHandle string `json:"new_handle"`
}

type SystemNoticePayload struct {
	Message string `json:"message"`
}

type PongPayload struct {
	Message string `json:"message"`
}

type CallInvitePayload struct {
	TargetHandle string `json:"target_handle"`
	Channel      string `json:"channel"`
}

type CallStatePayload struct {
	SessionID string             `json:"session_id"`
	Target    string             `json:"target"`
	Status    models.CallStatus  `json:"status"`
	Muted     bool               `json:"muted"`
	Note      string             `json:"note,omitempty"`
}
