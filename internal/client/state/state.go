package state

import (
	"sort"
	"time"

	"github.com/eisen/teamchat/internal/client/call"
	"github.com/eisen/teamchat/internal/shared/models"
)

type App struct {
	Handle            string
	ServerURL         string
	Workspace         string
	WorkspaceOwnerID  string
	Connected         bool
	StatusText        string
	Current           string
	Channels          []string
	Users             []string
	UserMeta          []UserEntry
	Messages          []models.Message
	Typing            map[string]time.Time
	Call              call.State
	Notification      string
	HighlightedHandle string
}

type UserEntry struct {
	ID      string
	Handle  string
	Online  bool
	Channel string
	Role    string
}

func New() App {
	return App{
		Typing: make(map[string]time.Time),
		Call:   call.InitialState(),
	}
}

func (a *App) SetChannels(channels []models.Channel) {
	a.Channels = a.Channels[:0]
	for _, c := range channels {
		a.Channels = append(a.Channels, c.Name)
	}
	sort.Strings(a.Channels)
}

func (a *App) SetUsers(users []models.User, presences map[string]bool) {
	a.SetUsersDetailed(users, nil, presences)
}

func (a *App) SetUsersDetailed(users []models.User, presenceList []models.Presence, presences map[string]bool) {
	a.Users = a.Users[:0]
	a.UserMeta = a.UserMeta[:0]
	channelByHandle := make(map[string]string, len(presenceList))
	for _, p := range presenceList {
		channelByHandle[p.Handle] = p.Channel
	}
	for _, u := range users {
		online := presences[u.Handle]
		suffix := ""
		if online {
			suffix = " *"
		}
		a.Users = append(a.Users, u.Handle+suffix)
		role := "Member"
		if a.WorkspaceOwnerID != "" && u.ID == a.WorkspaceOwnerID {
			role = "Owner"
		}
		a.UserMeta = append(a.UserMeta, UserEntry{
			ID:      u.ID,
			Handle:  u.Handle,
			Online:  online,
			Channel: channelByHandle[u.Handle],
			Role:    role,
		})
	}
	sort.Strings(a.Users)
	sort.Slice(a.UserMeta, func(i, j int) bool {
		return a.UserMeta[i].Handle < a.UserMeta[j].Handle
	})
}

func (a *App) UpsertMessage(msg models.Message) {
	a.Messages = append(a.Messages, msg)
	if len(a.Messages) > 500 {
		a.Messages = a.Messages[len(a.Messages)-500:]
	}
}
