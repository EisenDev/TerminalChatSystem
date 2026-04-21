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
	CreateMediaAsset(ctx context.Context, asset models.MediaAsset) (models.MediaAsset, error)
	GetMediaAsset(ctx context.Context, id string) (models.MediaAsset, error)
	CountMediaByKind(ctx context.Context, channelID string, kind models.MediaKind) (int, error)
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
		       , coalesce(ma.id::text, ''), coalesce(ma.kind, ''), coalesce('/pub/' || ma.id::text, '')
		from messages m
		join users u on u.id = m.user_id
		join channels c on c.id = m.channel_id
		left join media_assets ma on ma.message_id = m.id
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
		var mediaID, mediaKind, mediaURL string
		if err := rows.Scan(&msg.ID, &msg.WorkspaceID, &msg.ChannelID, &msg.ChannelName, &msg.UserID, &msg.UserHandle, &msg.Body, &msg.MessageType, &msg.CreatedAt, &mediaID, &mediaKind, &mediaURL); err != nil {
			return nil, err
		}
		msg.MediaID = mediaID
		msg.MediaKind = models.MediaKind(mediaKind)
		msg.MediaURL = mediaURL
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

func (s *Postgres) CreateMediaAsset(ctx context.Context, asset models.MediaAsset) (models.MediaAsset, error) {
	query := `
		insert into media_assets (workspace_id, channel_id, message_id, user_id, kind, object_key, file_name, content_type, byte_size)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning id::text, workspace_id::text, channel_id::text, message_id::text, user_id::text, kind, object_key, file_name, content_type, byte_size, created_at`
	var out models.MediaAsset
	err := s.db.QueryRow(ctx, query,
		asset.WorkspaceID, asset.ChannelID, asset.MessageID, asset.UserID, asset.Kind, asset.ObjectKey, asset.FileName, asset.ContentType, asset.ByteSize,
	).Scan(&out.ID, &out.WorkspaceID, &out.ChannelID, &out.MessageID, &out.UserID, &out.Kind, &out.ObjectKey, &out.FileName, &out.ContentType, &out.ByteSize, &out.CreatedAt)
	out.UserHandle = asset.UserHandle
	out.PublicURL = asset.PublicURL
	return out, err
}

func (s *Postgres) GetMediaAsset(ctx context.Context, id string) (models.MediaAsset, error) {
	query := `
		select ma.id::text, ma.workspace_id::text, ma.channel_id::text, ma.message_id::text, ma.user_id::text,
		       u.handle, ma.kind, ma.object_key, ma.file_name, ma.content_type, ma.byte_size, ma.created_at
		from media_assets ma
		join users u on u.id = ma.user_id
		where ma.id = $1`
	var out models.MediaAsset
	err := s.db.QueryRow(ctx, query, id).Scan(
		&out.ID, &out.WorkspaceID, &out.ChannelID, &out.MessageID, &out.UserID, &out.UserHandle,
		&out.Kind, &out.ObjectKey, &out.FileName, &out.ContentType, &out.ByteSize, &out.CreatedAt,
	)
	return out, err
}

func (s *Postgres) CountMediaByKind(ctx context.Context, channelID string, kind models.MediaKind) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `select count(*) from media_assets where channel_id = $1 and kind = $2`, channelID, kind).Scan(&count)
	return count, err
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

func findOrCreateUserByHandle(ctx context.Context, tx pgx.Tx, handle string) (models.User, error) {
	var user models.User
	err := tx.QueryRow(ctx, `
		insert into users (handle, display_name)
		values ($1, $1)
		on conflict (handle) do update
		set display_name = excluded.display_name
		returning id::text, handle, display_name, created_at`, handle).
		Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
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
		if user, err = findOrCreateUserByHandle(ctx, tx, handle); err != nil {
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
			insert into workspace_device_accounts (workspace_id, device_fingerprint, user_id)
			values ($1, $2, $3)
			on conflict (workspace_id, device_fingerprint) do update
			set user_id = excluded.user_id, last_seen_at = now()`, workspace.ID, fingerprint, user.ID); err != nil {
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
		from workspace_device_accounts da
		join users u on u.id = da.user_id
		where da.workspace_id = $1
		  and da.device_fingerprint = $2`,
		workspace.ID, fingerprint,
	).Scan(&user.ID, &user.Handle, &user.DisplayName, &user.CreatedAt)
	switch {
	case err == nil:
		result.ExistingDevice = true
	case errors.Is(err, pgx.ErrNoRows):
		if handle == "" {
			return JoinWorkspaceResult{}, fmt.Errorf("handle is required the first time this device joins %s", name)
		}
		if user, err = findOrCreateUserByHandle(ctx, tx, handle); err != nil {
			return JoinWorkspaceResult{}, fmt.Errorf("create device handle: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			insert into workspace_device_accounts (workspace_id, device_fingerprint, user_id)
			values ($1, $2, $3)
			on conflict (workspace_id, device_fingerprint) do update
			set user_id = excluded.user_id, last_seen_at = now()`, workspace.ID, fingerprint, user.ID); err != nil {
			return JoinWorkspaceResult{}, err
		}
	default:
		return JoinWorkspaceResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		update workspace_device_accounts
		set last_seen_at = now()
		where workspace_id = $1 and device_fingerprint = $2`, workspace.ID, fingerprint); err != nil {
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
