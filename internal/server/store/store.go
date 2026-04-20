package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/eisen/teamchat/internal/shared/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	EnsureUser(ctx context.Context, handle string) (models.User, error)
	EnsureWorkspace(ctx context.Context, name string) (models.Workspace, error)
	EnsureChannel(ctx context.Context, workspaceID, name string, kind models.ChannelKind) (models.Channel, error)
	AddWorkspaceMember(ctx context.Context, workspaceID, userID string) error
	AddChannelMember(ctx context.Context, channelID, userID string) error
	ListChannels(ctx context.Context, workspaceID string) ([]models.Channel, error)
	ListWorkspaceUsers(ctx context.Context, workspaceID string) ([]models.User, error)
	ListHistory(ctx context.Context, channelID string, limit int) ([]models.Message, error)
	SaveMessage(ctx context.Context, workspaceID, channelID string, user models.User, body string, messageType models.MessageType) (models.Message, error)
	UpdateUserHandle(ctx context.Context, userID, handle string) (models.User, error)
}

type Postgres struct {
	db *pgxpool.Pool
}

func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{db: db}
}

func (s *Postgres) EnsureUser(ctx context.Context, handle string) (models.User, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	query := `
		insert into users (handle, display_name)
		values ($1, $1)
		on conflict (handle) do update set display_name = excluded.display_name
		returning id::text, handle, display_name, created_at`
	var user models.User
	err := s.db.QueryRow(ctx, query, handle).Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt)
	return user, err
}

func (s *Postgres) EnsureWorkspace(ctx context.Context, name string) (models.Workspace, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	query := `
		insert into workspaces (name)
		values ($1)
		on conflict (name) do update set name = excluded.name
		returning id::text, name, created_at`
	var w models.Workspace
	err := s.db.QueryRow(ctx, query, name).Scan(&w.ID, &w.Name, &w.CreatedAt)
	return w, err
}

func (s *Postgres) EnsureChannel(ctx context.Context, workspaceID, name string, kind models.ChannelKind) (models.Channel, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	query := `
		insert into channels (workspace_id, name, kind)
		values ($1, $2, $3)
		on conflict (workspace_id, name) do update set kind = excluded.kind
		returning id::text, workspace_id::text, name, kind, created_at`
	var c models.Channel
	err := s.db.QueryRow(ctx, query, workspaceID, name, kind).Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Kind, &c.CreatedAt)
	return c, err
}

func (s *Postgres) AddWorkspaceMember(ctx context.Context, workspaceID, userID string) error {
	_, err := s.db.Exec(ctx, `
		insert into workspace_members (workspace_id, user_id)
		values ($1, $2)
		on conflict (workspace_id, user_id) do nothing`, workspaceID, userID)
	return err
}

func (s *Postgres) AddChannelMember(ctx context.Context, channelID, userID string) error {
	_, err := s.db.Exec(ctx, `
		insert into channel_members (channel_id, user_id)
		values ($1, $2)
		on conflict (channel_id, user_id) do nothing`, channelID, userID)
	return err
}

func (s *Postgres) ListChannels(ctx context.Context, workspaceID string) ([]models.Channel, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, workspace_id::text, name, kind, created_at
		from channels
		where workspace_id = $1
		order by kind, name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Channel
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Kind, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Postgres) ListWorkspaceUsers(ctx context.Context, workspaceID string) ([]models.User, error) {
	rows, err := s.db.Query(ctx, `
		select u.id::text, u.handle, u.display_name, u.created_at
		from users u
		join workspace_members wm on wm.user_id = u.id
		where wm.workspace_id = $1
		order by u.handle`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Handle, &u.DisplayName, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Postgres) ListHistory(ctx context.Context, channelID string, limit int) ([]models.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		select m.id::text, m.workspace_id::text, m.channel_id::text, c.name, m.user_id::text, u.handle, m.body, m.message_type, m.created_at
		from messages m
		join users u on u.id = m.user_id
		join channels c on c.id = m.channel_id
		where m.channel_id = $1
		order by m.created_at desc
		limit $2`, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reversed []models.Message
	for rows.Next() {
		var msg models.Message
		if err := rows.Scan(&msg.ID, &msg.WorkspaceID, &msg.ChannelID, &msg.ChannelName, &msg.UserID, &msg.UserHandle, &msg.Body, &msg.MessageType, &msg.CreatedAt); err != nil {
			return nil, err
		}
		reversed = append(reversed, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.Message, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		out = append(out, reversed[i])
	}
	return out, nil
}

func (s *Postgres) SaveMessage(ctx context.Context, workspaceID, channelID string, user models.User, body string, messageType models.MessageType) (models.Message, error) {
	query := `
		insert into messages (workspace_id, channel_id, user_id, body, message_type)
		values ($1, $2, $3, $4, $5)
		returning id::text, workspace_id::text, channel_id::text, user_id::text, body, message_type, created_at`
	var msg models.Message
	err := s.db.QueryRow(ctx, query, workspaceID, channelID, user.ID, body, messageType).Scan(
		&msg.ID, &msg.WorkspaceID, &msg.ChannelID, &msg.UserID, &msg.Body, &msg.MessageType, &msg.CreatedAt,
	)
	msg.UserHandle = user.Handle
	_ = s.db.QueryRow(ctx, `select name from channels where id = $1`, channelID).Scan(&msg.ChannelName)
	return msg, err
}

func (s *Postgres) UpdateUserHandle(ctx context.Context, userID, handle string) (models.User, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	query := `
		update users
		set handle = $2, display_name = $2
		where id = $1
		returning id::text, handle, display_name, created_at`
	var user models.User
	err := s.db.QueryRow(ctx, query, userID, handle).Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt)
	return user, err
}

func ScanChannelByName(row pgx.Row) (models.Channel, error) {
	var c models.Channel
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Kind, &c.CreatedAt)
	if err != nil {
		return models.Channel{}, fmt.Errorf("scan channel: %w", err)
	}
	return c, nil
}
