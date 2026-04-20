package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WorkspaceProfile struct {
	Handle    string    `json:"handle"`
	UpdatedAt time.Time `json:"updated_at"`
}

type dataFile struct {
	DeviceToken string                      `json:"device_token"`
	Workspaces  map[string]WorkspaceProfile `json:"workspaces"`
}

type Store struct {
	path string
	data dataFile
}

func Open() (*Store, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("config dir: %w", err)
	}
	path := filepath.Join(cfgDir, "teamchat", "profile.json")
	s := &Store{
		path: path,
		data: dataFile{
			Workspaces: make(map[string]WorkspaceProfile),
		},
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &s.data); err != nil {
			return nil, fmt.Errorf("read profile: %w", err)
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("open profile: %w", err)
	}
	if s.data.Workspaces == nil {
		s.data.Workspaces = make(map[string]WorkspaceProfile)
	}
	if s.data.DeviceToken == "" {
		s.data.DeviceToken, err = newToken()
		if err != nil {
			return nil, err
		}
		if err := s.Save(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) DeviceToken() string {
	return s.data.DeviceToken
}

func (s *Store) Lookup(serverURL, workspace, code string) (WorkspaceProfile, bool) {
	profile, ok := s.data.Workspaces[keyFor(serverURL, workspace, code)]
	return profile, ok
}

func (s *Store) Remember(serverURL, workspace, code, handle string) error {
	s.data.Workspaces[keyFor(serverURL, workspace, code)] = WorkspaceProfile{
		Handle:    strings.ToLower(strings.TrimSpace(handle)),
		UpdatedAt: time.Now().UTC(),
	}
	return s.Save()
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("mkdir profile dir: %w", err)
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	return nil
}

func keyFor(serverURL, workspace, code string) string {
	return strings.ToLower(strings.TrimSpace(serverURL)) + "|" +
		strings.ToLower(strings.TrimSpace(workspace)) + "|" +
		strings.TrimSpace(code)
}

func newToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate device token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
