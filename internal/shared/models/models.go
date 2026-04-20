package models

import "time"

type ChannelKind string

const (
	ChannelKindPublic ChannelKind = "public"
	ChannelKindDirect ChannelKind = "direct"
	ChannelKindSystem ChannelKind = "system"
)

type MessageType string

const (
	MessageTypeChat   MessageType = "chat"
	MessageTypeSystem MessageType = "system"
	MessageTypeEmote  MessageType = "emote"
)

type CallStatus string

const (
	CallStatusIdle     CallStatus = "idle"
	CallStatusIncoming CallStatus = "incoming"
	CallStatusRinging  CallStatus = "ringing"
	CallStatusActive   CallStatus = "active"
)

type User struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type Workspace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerHandle string    `json:"owner_handle,omitempty"`
	OwnerUserID string    `json:"owner_user_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Channel struct {
	ID          string      `json:"id"`
	WorkspaceID string      `json:"workspace_id"`
	Name        string      `json:"name"`
	Kind        ChannelKind `json:"kind"`
	CreatedAt   time.Time   `json:"created_at"`
}

type Message struct {
	ID          string      `json:"id"`
	WorkspaceID string      `json:"workspace_id"`
	ChannelID   string      `json:"channel_id"`
	ChannelName string      `json:"channel_name,omitempty"`
	UserID      string      `json:"user_id"`
	UserHandle  string      `json:"user_handle"`
	Body        string      `json:"body"`
	MessageType MessageType `json:"message_type"`
	CreatedAt   time.Time   `json:"created_at"`
}

type Presence struct {
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	Handle      string    `json:"handle"`
	Channel     string    `json:"channel,omitempty"`
	Online      bool      `json:"online"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

type CallSession struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	ChannelID   string     `json:"channel_id"`
	InitiatorID string     `json:"initiator_id"`
	Status      CallStatus `json:"status"`
	Muted       bool       `json:"muted"`
}
