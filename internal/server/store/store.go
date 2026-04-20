package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/eisen/teamchat/internal/shared/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	JoinWorkspace(ctx context.Context, req JoinWorkspaceRequest) (JoinWorkspaceResult, error)
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

type JoinWorkspaceRequest struct {
	Name              string
	Code              string
	RequestedHandle   string
	DeviceFingerprint string
}

type JoinWorkspaceResult struct {
	Workspace      models.Workspace
	User           models.User
	NewWorkspace   bool
	ExistingDevice bool
}

func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{db: db}
}

func (s *Postgres) JoinWorkspace(ctx context.Context, req JoinWorkspaceRequest) (JoinWorkspaceResult, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	code := strings.TrimSpace(req.Code)
	handle := strings.ToLower(strings.TrimSpace(req.RequestedHandle))
	fingerprint := strings.TrimSpace(req.DeviceFingerprint)
	if code == "" {
		return JoinWorkspaceResult{}, fmt.Errorf("workspace code is required")
	}
	if fingerprint == "" {
		return JoinWorkspaceResult{}, fmt.Errorf("device fingerprint is required")
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return JoinWorkspaceResult{}, err
	}
	defer tx.Rollback(ctx)

	result, err := s.joinWorkspaceTx(ctx, tx, name, code, handle, fingerprint)
	if err != nil {
		return JoinWorkspaceResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return JoinWorkspaceResult{}, err
	}
	return result, nil
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
		  and m.created_at >= now() - interval '7 days'
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
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.User{}, err
	}
	defer tx.Rollback(ctx)

	query := `
		update users
		set handle = $2, display_name = $2
		where id = $1
		returning id::text, handle, display_name, created_at`
	var user models.User
	if err := tx.QueryRow(ctx, query, userID, handle).Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt); err != nil {
		return models.User{}, err
	}
	if _, err := tx.Exec(ctx, `
		update workspaces
		set owner_handle = $2
		where owner_user_id = $1`, userID, handle); err != nil {
		return models.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func ScanChannelByName(row pgx.Row) (models.Channel, error) {
	var c models.Channel
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Kind, &c.CreatedAt)
	if err != nil {
		return models.Channel{}, fmt.Errorf("scan channel: %w", err)
	}
	return c, nil
}

func (s *Postgres) joinWorkspaceTx(ctx context.Context, tx pgx.Tx, name, code, handle, fingerprint string) (JoinWorkspaceResult, error) {
	var (
		result    JoinWorkspaceResult
		workspace models.Workspace
		user      models.User
	)

	err := tx.QueryRow(ctx, `
		select id::text, name, owner_handle, coalesce(owner_user_id::text, ''), created_at
		from workspaces
		where name = $1`, name).Scan(&workspace.ID, &workspace.Name, &workspace.OwnerHandle, &workspace.OwnerUserID, &workspace.CreatedAt)
	switch {
	case err == nil:
	case errors.Is(err, pgx.ErrNoRows):
		if handle == "" {
			return JoinWorkspaceResult{}, fmt.Errorf("handle is required the first time this device joins a workspace")
		}
		if err := tx.QueryRow(ctx, `
			insert into users (handle, display_name)
			values ($1, $1)
			returning id::text, handle, display_name, created_at`, handle).Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt); err != nil {
			return JoinWorkspaceResult{}, fmt.Errorf("create owner handle: %w", err)
		}
		err = tx.QueryRow(ctx, `
			insert into workspaces (name, join_code, owner_handle, owner_user_id)
			values ($1, $2, $3, $4)
			returning id::text, name, owner_handle, coalesce(owner_user_id::text, ''), created_at`,
			name, code, user.Handle, user.ID,
		).Scan(&workspace.ID, &workspace.Name, &workspace.OwnerHandle, &workspace.OwnerUserID, &workspace.CreatedAt)
		if err != nil {
			return JoinWorkspaceResult{}, err
		}
		if _, err := tx.Exec(ctx, `
			insert into device_accounts (device_fingerprint, user_id)
			values ($1, $2)
			on conflict (device_fingerprint) do update
			set last_seen_at = now()`, fingerprint, user.ID); err != nil {
			return JoinWorkspaceResult{}, err
		}
		if _, err := tx.Exec(ctx, `
			insert into workspace_members (workspace_id, user_id)
			values ($1, $2)
			on conflict (workspace_id, user_id) do nothing`, workspace.ID, user.ID); err != nil {
			return JoinWorkspaceResult{}, err
		}
		return JoinWorkspaceResult{
			Workspace:    workspace,
			User:         user,
			NewWorkspace: true,
		}, nil
	default:
		return JoinWorkspaceResult{}, err
	}

	var storedCode string
	if err := tx.QueryRow(ctx, `select join_code from workspaces where id = $1`, workspace.ID).Scan(&storedCode); err != nil {
		return JoinWorkspaceResult{}, err
	}
	if storedCode != code {
		return JoinWorkspaceResult{}, fmt.Errorf("invalid workspace name or code")
	}

	err = tx.QueryRow(ctx, `
		select u.id::text, u.handle, u.display_name, u.created_at
		from device_accounts da
		join users u on u.id = da.user_id
		where da.device_fingerprint = $1`,
		fingerprint,
	).Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt)
	switch {
	case err == nil:
		result.ExistingDevice = true
	case errors.Is(err, pgx.ErrNoRows):
		if handle == "" {
			return JoinWorkspaceResult{}, fmt.Errorf("handle is required the first time this device joins %s", name)
		}
		if err := tx.QueryRow(ctx, `
			insert into users (handle, display_name)
			values ($1, $1)
			returning id::text, handle, display_name, created_at`, handle).Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt); err != nil {
			return JoinWorkspaceResult{}, fmt.Errorf("create device handle: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			insert into device_accounts (device_fingerprint, user_id)
			values ($1, $2)
			on conflict (device_fingerprint) do update
			set user_id = excluded.user_id, last_seen_at = now()`, fingerprint, user.ID); err != nil {
			return JoinWorkspaceResult{}, err
		}
	default:
		return JoinWorkspaceResult{}, err
	}

	if _, err := tx.Exec(ctx, `update device_accounts set last_seen_at = now() where device_fingerprint = $1`, fingerprint); err != nil {
		return JoinWorkspaceResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		insert into workspace_members (workspace_id, user_id)
		values ($1, $2)
		on conflict (workspace_id, user_id) do nothing`, workspace.ID, user.ID); err != nil {
		return JoinWorkspaceResult{}, err
	}
	result.Workspace = workspace
	result.User = user
	return result, nil
}
