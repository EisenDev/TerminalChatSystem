package ui

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eisen/teamchat/internal/client/notify"
	"github.com/eisen/teamchat/internal/client/profile"
	"github.com/eisen/teamchat/internal/client/state"
	clientws "github.com/eisen/teamchat/internal/client/ws"
	"github.com/eisen/teamchat/internal/shared/buildinfo"
	"github.com/eisen/teamchat/internal/shared/config"
	"github.com/eisen/teamchat/internal/shared/models"
	"github.com/eisen/teamchat/internal/shared/protocol"
)

type phase int

const (
	phaseSetup phase = iota
	phaseChat
)

type frameTickMsg time.Time
type autoConnectMsg struct{}
type updateResultMsg struct{ Message string }

type pingEffectState struct {
	From      string
	Effect    string
	Scope     string
	Target    string
	StartedAt time.Time
	EndsAt    time.Time
}

type keyMap struct {
	Help      key.Binding
	Reconnect key.Binding
	Prev      key.Binding
	Next      key.Binding
	Quit      key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Reconnect, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Help, k.Reconnect, k.Prev, k.Next, k.Quit}}
}

type Model struct {
	cfg    config.Client
	logger *slog.Logger

	phase             phase
	width             int
	height            int
	help              help.Model
	keys              keyMap
	app               state.App
	ws                *clientws.Manager
	presence          map[string]models.Presence
	ctx               context.Context
	cancel            context.CancelFunc
	setupStep         int
	setupInputs       []textinput.Model
	input             textarea.Model
	viewport          viewport.Model
	activePane        int
	showHelp          bool
	showEmotePicker   bool
	showLobbyBrowser  bool
	showCommandPanel  bool
	commandPadView    string
	userRoster        []models.User
	channelCursor     int
	emoteCursor       int
	emoteFrame        int
	lobbyCursor       int
	highlightUntil    map[string]time.Time
	messagesByChannel map[string][]models.Message
	clearedMessages   map[string][]models.Message
	inputHistory      []string
	inputHistoryIndex int
	inputDraft        string
	effectsEnabled    bool
	mutedEffectUsers  map[string]bool
	activeEffect      *pingEffectState
	profiles          *profile.Store
	savedLobbies      []profile.SavedLobby
	deviceToken       string
}

type wsEventMsg clientws.Event

func NewModel(cfg config.Client, logger *slog.Logger) Model {
	search := textinput.New()
	search.Placeholder = "search lobby (optional)"
	search.Focus()

	lobby := textinput.New()
	lobby.Placeholder = "lobby name"
	lobby.SetValue(cfg.Workspace)

	lobbyCode := textinput.New()
	lobbyCode.Placeholder = "lobby code"
	lobbyCode.SetValue(cfg.WorkspaceCode)

	username := textinput.New()
	username.Placeholder = "username"
	username.SetValue(cfg.DefaultHandle)

	input := textarea.New()
	input.Placeholder = "type a message or /help"
	input.Focus()
	input.SetHeight(3)

	vp := viewport.New(80, 20)
	ctx, cancel := context.WithCancel(context.Background())
	manager := clientws.NewManager(logger, cfg)
	profiles, err := profile.Open()
	if err != nil {
		logger.Warn("open client profile", "error", err)
	}

	model := Model{
		cfg:      cfg,
		logger:   logger,
		phase:    phaseSetup,
		help:     help.New(),
		keys:     defaultKeys(),
		app:      state.New(),
		ws:       manager,
		presence: make(map[string]models.Presence),
		ctx:      ctx,
		cancel:   cancel,
		setupInputs: []textinput.Model{
			search, lobby, lobbyCode, username,
		},
		input:             input,
		viewport:          vp,
		highlightUntil:    make(map[string]time.Time),
		messagesByChannel: make(map[string][]models.Message),
		clearedMessages:   make(map[string][]models.Message),
		effectsEnabled:    true,
		mutedEffectUsers:  make(map[string]bool),
		inputHistoryIndex: -1,
		profiles:          profiles,
	}
	if profiles != nil {
		model.deviceToken = profiles.DeviceToken()
		model.savedLobbies = profiles.List()
		model.prefillRememberedHandle()
	} else {
		model.deviceToken = fmt.Sprintf("ephemeral-%d", time.Now().UnixNano())
	}
	return model
}

func defaultKeys() keyMap {
	return keyMap{
		Help:      key.NewBinding(key.WithKeys("f1"), key.WithHelp("f1", "help")),
		Reconnect: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reconnect")),
		Prev:      key.NewBinding(key.WithKeys("shift+tab", "left"), key.WithHelp("left", "prev pane")),
		Next:      key.NewBinding(key.WithKeys("tab", "right"), key.WithHelp("right", "next pane")),
		Quit:      key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink, tickFrame()}
	if m.shouldAutoConnect() {
		cmds = append(cmds, func() tea.Msg { return autoConnectMsg{} })
	}
	return tea.Batch(cmds...)
}

func tickFrame() tea.Cmd {
	return tea.Tick(180*time.Millisecond, func(t time.Time) tea.Msg {
		return frameTickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		m.refreshViewport()
		return m, nil

	case frameTickMsg:
		m.emoteFrame++
		if m.activeEffect != nil && time.Now().After(m.activeEffect.EndsAt) {
			m.activeEffect = nil
		}
		m.refreshViewport()
		return m, tickFrame()

	case autoConnectMsg:
		if m.phase == phaseSetup && m.shouldAutoConnect() {
			m.connectFromSetup()
			return m, waitForWSEvent(m.ws.Events())
		}
		return m, nil
	case updateResultMsg:
		m.app.Notification = msg.Message
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			m.cancel()
			m.ws.Close()
			return m, tea.Quit
		}
		if key.Matches(msg, m.keys.Help) {
			m.showCommandPanel = !m.showCommandPanel
			m.showHelp = m.showCommandPanel
			if !m.showCommandPanel {
				m.showEmotePicker = false
			}
			return m, nil
		}
		if key.Matches(msg, m.keys.Reconnect) && m.phase == phaseChat {
			m.reconnect()
			m.app.Notification = "reconnect requested"
			return m, waitForWSEvent(m.ws.Events())
		}
		if m.phase == phaseSetup {
			return m.updateSetup(msg)
		}
		if m.showEmotePicker {
			return m.updateEmotePicker(msg)
		}
		return m.updateChatKeys(msg)

	case tea.MouseMsg:
		if m.phase == phaseChat {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case wsEventMsg:
		return m.handleWSEvent(clientws.Event(msg))
	}

	if m.phase == phaseSetup {
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.phase == phaseSetup {
		return m.viewSetup()
	}
	return m.viewChat()
}

func (m Model) updateSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showLobbyBrowser {
		switch msg.Type {
		case tea.KeyEsc:
			m.showLobbyBrowser = false
			m.setupInputs[m.setupStep].SetValue("")
			return m, nil
		case tea.KeyUp:
			if m.lobbyCursor > 0 {
				m.lobbyCursor--
			}
			return m, nil
		case tea.KeyDown:
			if m.lobbyCursor < len(m.savedLobbies)-1 {
				m.lobbyCursor++
			}
			return m, nil
		case tea.KeyEnter:
			if len(m.savedLobbies) == 0 {
				m.showLobbyBrowser = false
				return m, nil
			}
			selected := m.savedLobbies[m.lobbyCursor]
			m.setupInputs[1].SetValue(selected.LobbyName)
			m.setupInputs[2].SetValue(selected.LobbyCode)
			m.setupInputs[3].SetValue(selected.Handle)
			m.showLobbyBrowser = false
			m.setupStep = 2
			m.connectFromSetup()
			return m, waitForWSEvent(m.ws.Events())
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEnter:
		currentValue := strings.TrimSpace(m.setupInputs[m.setupStep].Value())
		if m.setupStep == 0 && currentValue == "/list" {
			if len(m.savedLobbies) == 0 {
				m.app.Notification = "no saved lobbies on this device yet"
				m.setupInputs[0].SetValue("")
				return m, nil
			}
			m.showLobbyBrowser = true
			m.lobbyCursor = 0
			return m, nil
		}
		if m.setupStep == 0 {
			m.applySearchLobby()
			m.setupInputs[m.setupStep].Blur()
			m.setupStep++
			m.setupInputs[m.setupStep].Focus()
			return m, textinput.Blink
		}
		if m.setupStep == 1 {
			m.setupInputs[m.setupStep].Blur()
			m.setupStep = 2
			m.setupInputs[m.setupStep].Focus()
			return m, textinput.Blink
		}
		if m.setupStep == 2 {
			if remembered, ok := m.lookupRememberedHandle(); ok && remembered.Handle != "" {
				m.setupInputs[3].SetValue(remembered.Handle)
				m.connectFromSetup()
				return m, waitForWSEvent(m.ws.Events())
			}
			m.setupInputs[m.setupStep].Blur()
			m.setupStep = 3
			m.setupInputs[m.setupStep].Focus()
			return m, textinput.Blink
		}
		m.connectFromSetup()
		return m, waitForWSEvent(m.ws.Events())
	case tea.KeyTab, tea.KeyShiftTab, tea.KeyUp, tea.KeyDown:
		visibleInputs := 3
		if m.setupStep >= 3 || strings.TrimSpace(m.setupInputs[3].Value()) != "" {
			visibleInputs = 4
		}
		delta := 1
		if msg.Type == tea.KeyShiftTab || msg.Type == tea.KeyUp {
			delta = -1
		}
		m.setupInputs[m.setupStep].Blur()
		m.setupStep = (m.setupStep + delta + visibleInputs) % visibleInputs
		m.setupInputs[m.setupStep].Focus()
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.setupInputs[m.setupStep], cmd = m.setupInputs[m.setupStep].Update(msg)
	m.prefillRememberedHandle()
	return m, cmd
}

func (m *Model) reconnect() {
	m.ws.Close()
	m.ws = clientws.NewManager(m.logger, m.cfg)
	m.ws.Configure(clientws.Session{
		Handle:      m.app.Handle,
		Workspace:   m.app.Workspace,
		Code:        m.cfg.WorkspaceCode,
		Channel:     m.app.Current,
		DeviceToken: m.deviceToken,
	})
	m.ws.Start(m.ctx)
}

func (m *Model) connectFromSetup() {
	m.app.ServerURL = strings.TrimSpace(m.cfg.ServerURL)
	m.app.Workspace = strings.TrimSpace(m.setupInputs[1].Value())
	m.cfg.WorkspaceCode = strings.TrimSpace(m.setupInputs[2].Value())
	m.app.Handle = strings.TrimSpace(m.setupInputs[3].Value())
	if m.app.Handle == "" {
		if remembered, ok := m.lookupRememberedHandle(); ok {
			m.app.Handle = remembered.Handle
			m.setupInputs[3].SetValue(remembered.Handle)
		}
	}
	m.app.Current = m.cfg.DefaultChannel
	m.cfg.ServerURL = m.app.ServerURL
	m.reconnect()
	m.phase = phaseChat
	m.app.StatusText = "connecting"
}

func (m Model) updateChatKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.showEmotePicker {
			m.showEmotePicker = false
			if m.commandPadView == "emotes" {
				m.commandPadView = ""
			}
			return m, nil
		}
		if m.showCommandPanel {
			m.showCommandPanel = false
			m.showHelp = false
			if m.commandPadView == "commands" {
				m.commandPadView = ""
			}
			return m, nil
		}
	case tea.KeyPgUp:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case tea.KeyPgDown:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case tea.KeyHome:
		m.viewport.GotoTop()
		return m, nil
	case tea.KeyEnd:
		m.viewport.GotoBottom()
		return m, nil
	case tea.KeyUp:
		m.recallInput(-1)
		return m, nil
	case tea.KeyDown:
		m.recallInput(1)
		return m, nil
	case tea.KeyCtrlV:
		m.app.Notification = "reading clipboard..."
		return m, m.runClipboardUpload()
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		m.pushInputHistory(text)
		if cmd, note, ok := m.autoUploadInput(text); ok {
			m.app.Notification = note
			m.input.Reset()
			return m, cmd
		}
		if strings.HasPrefix(text, "/") {
			cmd := m.handleSlashCommand(text)
			m.input.Reset()
			return m, cmd
		}
		text = normalizeOutgoingMessage(text)
		_ = m.ws.Send(protocol.ClientSendMessage, protocol.SendMessagePayload{
			Channel: m.app.Current,
			Body:    text,
		})
		m.input.Reset()
		return m, nil
	}

	switch msg.String() {
	case "ctrl+u", "ctrl+b":
		m.viewport.LineUp(max(1, m.viewport.Height/2))
		return m, nil
	case "ctrl+d", "ctrl+f":
		m.viewport.LineDown(max(1, m.viewport.Height/2))
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) pushInputHistory(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if n := len(m.inputHistory); n > 0 && m.inputHistory[n-1] == text {
		m.inputHistoryIndex = -1
		m.inputDraft = ""
		return
	}
	m.inputHistory = append(m.inputHistory, text)
	if len(m.inputHistory) > 100 {
		m.inputHistory = m.inputHistory[len(m.inputHistory)-100:]
	}
	m.inputHistoryIndex = -1
	m.inputDraft = ""
}

func (m *Model) recallInput(delta int) {
	if len(m.inputHistory) == 0 {
		return
	}
	if delta < 0 {
		if m.inputHistoryIndex == -1 {
			m.inputDraft = m.input.Value()
			m.inputHistoryIndex = len(m.inputHistory) - 1
		} else if m.inputHistoryIndex > 0 {
			m.inputHistoryIndex--
		}
	} else {
		if m.inputHistoryIndex == -1 {
			return
		}
		if m.inputHistoryIndex < len(m.inputHistory)-1 {
			m.inputHistoryIndex++
		} else {
			m.inputHistoryIndex = -1
			m.input.SetValue(m.inputDraft)
			m.input.SetCursor(len([]rune(m.inputDraft)))
			return
		}
	}
	value := m.inputHistory[m.inputHistoryIndex]
	m.input.SetValue(value)
	m.input.SetCursor(len([]rune(value)))
}

func (m Model) updateEmotePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pageSize := 10
	pageStart := (m.emoteCursor / pageSize) * pageSize
	switch msg.Type {
	case tea.KeyEsc:
		m.showEmotePicker = false
		return m, nil
	case tea.KeyUp:
		if m.emoteCursor > 0 {
			m.emoteCursor--
		}
	case tea.KeyDown:
		if m.emoteCursor < len(emoteCatalog)-1 {
			m.emoteCursor++
		}
	case tea.KeyLeft, tea.KeyPgUp:
		if m.emoteCursor-pageSize >= 0 {
			m.emoteCursor -= pageSize
		} else {
			m.emoteCursor = 0
		}
	case tea.KeyRight, tea.KeyPgDown:
		if m.emoteCursor+pageSize < len(emoteCatalog) {
			m.emoteCursor += pageSize
		} else if len(emoteCatalog) > 0 {
			m.emoteCursor = len(emoteCatalog) - 1
		}
	case tea.KeyEnter:
		m.sendEmote(emoteCatalog[m.emoteCursor].ID)
		m.showEmotePicker = false
	}
	switch msg.String() {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
		slot := 10
		if msg.String() != "0" {
			slot, _ = strconv.Atoi(msg.String())
		}
		index := pageStart + slot - 1
		if index >= 0 && index < len(emoteCatalog) {
			m.sendEmote(emoteCatalog[index].ID)
			m.showEmotePicker = false
		}
	}
	return m, nil
}

func (m *Model) sendEmote(id string) {
	_ = m.ws.Send(protocol.ClientSendEmote, protocol.SendEmotePayload{
		Channel: m.app.Current,
		EmoteID: id,
	})
	m.app.Notification = "emote sent: " + id
}

func (m *Model) handleSlashCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "/join":
		if len(fields) < 2 {
			m.app.Notification = "usage: /join <channel>"
			return nil
		}
		target := strings.TrimPrefix(fields[1], "#")
		m.app.Current = target
		_ = m.ws.Send(protocol.ClientJoinChannel, protocol.JoinChannelPayload{Channel: target})
		m.refreshViewport()
	case "/dm":
		if len(fields) < 2 {
			m.app.Notification = "usage: /dm <user>"
			return nil
		}
		target := strings.ToLower(strings.TrimSpace(fields[1]))
		if target == m.app.Handle {
			m.app.Notification = "cannot DM yourself"
			return nil
		}
		channel := dmChannelName(m.app.Handle, target)
		m.app.Current = channel
		_ = m.ws.Send(protocol.ClientJoinChannel, protocol.JoinChannelPayload{Channel: channel})
		m.app.Notification = "dm scaffold joined #" + channel
		m.refreshViewport()
	case "/users":
		_ = m.ws.Send(protocol.ClientRequestUsers, struct{}{})
		m.app.Notification = formatUsersSummary(m.app.UserMeta)
	case "/channels":
		_ = m.ws.Send(protocol.ClientRequestChannels, struct{}{})
		m.app.Notification = strings.Join(prefixList("#", m.app.Channels), ", ")
	case "/ping":
		target, effect, durationMS, err := parsePingCommand(fields)
		if err != nil {
			m.app.Notification = err.Error()
			return nil
		}
		if target == "all" {
			_ = m.ws.Send(protocol.ClientPingAll, protocol.PingPayload{Handle: "all", Effect: effect, Scope: "all", DurationMS: durationMS})
		} else {
			_ = m.ws.Send(protocol.ClientPingUser, protocol.PingPayload{Handle: target, Effect: effect, Scope: "user", DurationMS: durationMS})
		}
	case "/effects":
		if len(fields) < 2 {
			m.app.Notification = "usage: /effects <on|off>"
			return nil
		}
		switch strings.ToLower(fields[1]) {
		case "on":
			m.effectsEnabled = true
			m.app.Notification = "effects enabled"
		case "off":
			m.effectsEnabled = false
			m.activeEffect = nil
			m.app.Notification = "effects disabled"
		default:
			m.app.Notification = "usage: /effects <on|off>"
		}
	case "/muteeffects":
		if len(fields) < 2 {
			m.app.Notification = "usage: /muteeffects <handle>"
			return nil
		}
		target := strings.ToLower(strings.TrimSpace(fields[1]))
		m.mutedEffectUsers[target] = !m.mutedEffectUsers[target]
		if m.mutedEffectUsers[target] {
			m.app.Notification = "muted effects from " + target
		} else {
			delete(m.mutedEffectUsers, target)
			m.app.Notification = "unmuted effects from " + target
		}
	case "/chandle":
		if len(fields) < 2 {
			m.app.Notification = "usage: /chandle <new_handle>"
			return nil
		}
		_ = m.ws.Send(protocol.ClientChangeHandle, protocol.ChangeHandlePayload{Handle: fields[1]})
	case "/emote":
		if len(fields) > 1 {
			if n, err := strconv.Atoi(fields[1]); err == nil {
				if item, ok := emoteByNumber(n); ok {
					m.sendEmote(item.ID)
					return nil
				}
			}
		}
		m.showEmotePicker = true
		m.commandPadView = "emotes"
	case "/me":
		if len(fields) < 2 {
			m.app.Notification = "usage: /me <action>"
			return nil
		}
		body := "* " + m.app.Handle + " " + strings.Join(fields[1:], " ") + " *"
		_ = m.ws.Send(protocol.ClientSendMessage, protocol.SendMessagePayload{Channel: m.app.Current, Body: body})
	case "/clear":
		if current := m.messagesByChannel[m.app.Current]; len(current) > 0 {
			m.clearedMessages[m.app.Current] = append([]models.Message(nil), current...)
		}
		delete(m.messagesByChannel, m.app.Current)
		m.refreshViewport()
		m.app.Notification = "cleared local view for #" + m.app.Current
	case "/restore":
		if len(fields) < 2 || fields[1] != "--message" {
			m.app.Notification = "usage: /restore --message"
			return nil
		}
		saved := m.clearedMessages[m.app.Current]
		if len(saved) == 0 {
			m.app.Notification = "no cleared messages to restore"
			return nil
		}
		m.messagesByChannel[m.app.Current] = append([]models.Message(nil), saved...)
		delete(m.clearedMessages, m.app.Current)
		m.refreshViewport()
		m.app.Notification = "restored local messages for #" + m.app.Current
	case "/image", "/video", "/file":
		if len(fields) < 2 {
			m.app.Notification = "usage: " + fields[0] + " <path>"
			return nil
		}
		path := strings.Join(fields[1:], " ")
		m.app.Notification = "uploading " + path + "..."
		return m.runMediaUpload(fields[0], path)
	case "/paste":
		m.app.Notification = "reading clipboard..."
		return m.runClipboardUpload()
	case "/update":
		m.app.Notification = "updating termichat..."
		return runSelfUpdate()
	case "/call":
		if len(fields) < 2 {
			m.app.Notification = "usage: /call <user>"
			return nil
		}
		target := fields[1]
		m.app.Call.Target = target
		m.app.Call.Status = models.CallStatusRinging
		m.app.Call.Note = "voice scaffold only"
		_ = m.ws.Send(protocol.ClientCallInvite, protocol.CallInvitePayload{TargetHandle: target, Channel: m.app.Current})
	case "/mute":
		m.app.Call.Muted = !m.app.Call.Muted
		_ = m.ws.Send(protocol.ClientMuteStateChanged, protocol.CallStatePayload{Target: m.app.Call.Target, Muted: m.app.Call.Muted, Status: m.app.Call.Status})
	case "/hangup":
		m.app.Call.Status = models.CallStatusIdle
		_ = m.ws.Send(protocol.ClientCallHangup, protocol.CallStatePayload{Target: m.app.Call.Target, Status: m.app.Call.Status})
	case "/quit":
		m.cancel()
		m.ws.Close()
		return tea.Quit
	case "/help":
		m.showHelp = true
		m.showCommandPanel = true
		m.commandPadView = "commands"
	case "/commands":
		if len(fields) == 2 && fields[1] == "--help" {
			m.showCommandPanel = true
			m.showHelp = true
			m.commandPadView = "commands"
			m.app.Notification = "commands opened"
			return nil
		}
		m.app.Notification = "usage: /commands --help"
	case "/back":
		m.showHelp = false
		m.showEmotePicker = false
		m.showCommandPanel = false
		m.commandPadView = ""
		m.showLobbyBrowser = false
		m.phase = phaseSetup
		m.setupStep = 0
		for i := range m.setupInputs {
			m.setupInputs[i].Blur()
		}
		m.setupInputs[0].Focus()
		m.ws.Close()
		m.ws = clientws.NewManager(m.logger, m.cfg)
		m.app.StatusText = "disconnected"
		m.app.Notification = "returned to lobby select"
		return nil
	default:
		m.app.Notification = "unknown command"
	}
	return nil
}

func (m Model) handleWSEvent(evt clientws.Event) (tea.Model, tea.Cmd) {
	if evt.Status != nil {
		m.app.Connected = evt.Status.Connected
		m.app.StatusText = evt.Status.Message
		if evt.Err != nil {
			m.app.Notification = evt.Err.Error()
		}
		return m, waitForWSEvent(m.ws.Events())
	}
	if evt.Err != nil {
		m.app.StatusText = "reconnecting"
		m.app.Notification = evt.Err.Error()
		return m, waitForWSEvent(m.ws.Events())
	}
	if evt.Envelope == nil {
		return m, waitForWSEvent(m.ws.Events())
	}

	switch evt.Envelope.Type {
	case protocol.ServerIdentified:
		payload, _ := protocol.DecodePayload[protocol.IdentifiedPayload](*evt.Envelope)
		m.app.Handle = payload.User.Handle
		m.setupInputs[3].SetValue(payload.User.Handle)
		if m.profiles != nil && m.app.ServerURL != "" && m.app.Workspace != "" && m.cfg.WorkspaceCode != "" && payload.User.Handle != "" {
			if err := m.profiles.Remember(m.app.ServerURL, m.app.Workspace, m.cfg.WorkspaceCode, payload.User.Handle); err != nil {
				m.logger.Warn("remember workspace profile", "error", err)
			}
			m.savedLobbies = m.profiles.List()
		}
	case protocol.ServerWorkspaceJoined:
		payload, _ := protocol.DecodePayload[protocol.WorkspaceJoinedPayload](*evt.Envelope)
		m.app.Workspace = payload.Workspace.Name
		m.app.WorkspaceOwnerID = payload.Workspace.OwnerUserID
		m.app.Current = payload.CurrentChannel
		m.userRoster = payload.Users
		m.app.SetChannels(payload.Channels)
		m.syncChannelCursor()
		m.app.Notification = "workspace joined"
	case protocol.ServerUsersList:
		payload, _ := protocol.DecodePayload[protocol.UsersListPayload](*evt.Envelope)
		m.userRoster = payload.Users
		m.rebuildUsers()
	case protocol.ServerChannelsList:
		payload, _ := protocol.DecodePayload[protocol.ChannelsListPayload](*evt.Envelope)
		m.app.SetChannels(payload.Channels)
		m.syncChannelCursor()
	case protocol.ServerChannelJoined:
		payload, _ := protocol.DecodePayload[protocol.ChannelJoinedPayload](*evt.Envelope)
		m.app.Current = payload.Channel.Name
		m.syncChannelCursor()
		m.app.Notification = "joined #" + payload.Channel.Name
	case protocol.ServerHistoryBatch:
		payload, _ := protocol.DecodePayload[protocol.HistoryBatchPayload](*evt.Envelope)
		m.messagesByChannel[payload.Channel] = append([]models.Message(nil), payload.Messages...)
	case protocol.ServerMessageNew, protocol.ServerEmoteNew:
		payload, _ := protocol.DecodePayload[models.Message](*evt.Envelope)
		channel := payload.ChannelName
		if channel == "" {
			channel = m.app.Current
		}
		m.messagesByChannel[channel] = appendMessage(m.messagesByChannel[channel], payload)
		if payload.UserHandle != "" && payload.UserHandle != m.app.Handle && messageMentionsHandle(payload.Body, m.app.Handle) {
			m.highlightUntil[m.app.Handle] = time.Now().Add(6 * time.Second)
			m.app.Notification = payload.UserHandle + " mentioned you"
			notify.Send("Notification from "+payload.UserHandle, "Open TermiChat")
		}
	case protocol.ServerPresenceUpdate:
		payload, _ := protocol.DecodePayload[protocol.PresenceUpdatePayload](*evt.Envelope)
		for _, p := range payload.Presences {
			m.presence[p.Handle] = p
		}
		m.rebuildUsers()
	case protocol.ServerUserJoined:
		payload, _ := protocol.DecodePayload[protocol.UserEventPayload](*evt.Envelope)
		m.app.Notification = payload.Handle + " joined the lobby"
		_ = m.ws.Send(protocol.ClientRequestUsers, struct{}{})
	case protocol.ServerUserLeft:
		payload, _ := protocol.DecodePayload[protocol.UserEventPayload](*evt.Envelope)
		m.app.Notification = payload.Handle + " left the lobby"
		_ = m.ws.Send(protocol.ClientRequestUsers, struct{}{})
	case protocol.ServerTypingUpdate:
		payload, _ := protocol.DecodePayload[protocol.TypingUpdatePayload](*evt.Envelope)
		if payload.Active {
			m.app.Typing[payload.Handle] = time.Now().Add(4 * time.Second)
		} else {
			delete(m.app.Typing, payload.Handle)
		}
	case protocol.ServerPingReceived:
		payload, _ := protocol.DecodePayload[protocol.PingReceivedPayload](*evt.Envelope)
		m.app.HighlightedHandle = payload.From
		m.highlightUntil[payload.From] = time.Now().Add(6 * time.Second)
		m.app.Notification = pingNotification(payload)
		notify.Send("Notification from "+payload.From, "Open TermiChat")
	case protocol.ServerPingEffect:
		payload, _ := protocol.DecodePayload[protocol.PingEffectPayload](*evt.Envelope)
		if m.effectsEnabled && !m.mutedEffectUsers[payload.From] {
			now := time.Now()
			m.activeEffect = &pingEffectState{
				From:      payload.From,
				Effect:    payload.Effect,
				Scope:     payload.Scope,
				Target:    payload.Target,
				StartedAt: now,
				EndsAt:    now.Add(time.Duration(payload.DurationMS) * time.Millisecond),
			}
		}
		m.app.Notification = fmt.Sprintf("%s launched %s", payload.From, payload.Effect)
		notify.Send("Notification from "+payload.From, "Open TermiChat")
	case protocol.ServerHandleChanged:
		payload, _ := protocol.DecodePayload[protocol.HandleChangedPayload](*evt.Envelope)
		if m.app.Handle == payload.OldHandle {
			m.app.Handle = payload.NewHandle
		}
		m.app.Notification = fmt.Sprintf("%s is now %s", payload.OldHandle, payload.NewHandle)
	case protocol.ServerSystemNotice:
		payload, _ := protocol.DecodePayload[protocol.SystemNoticePayload](*evt.Envelope)
		m.app.Notification = payload.Message
	case protocol.ServerError:
		payload, _ := protocol.DecodePayload[protocol.ErrorPayload](*evt.Envelope)
		m.app.Notification = payload.Message
	case protocol.ServerCallStateUpdate, protocol.ServerCallInvitation:
		payload, _ := protocol.DecodePayload[protocol.CallStatePayload](*evt.Envelope)
		m.app.Call.Target = payload.Target
		m.app.Call.Status = payload.Status
		m.app.Call.Muted = payload.Muted
		m.app.Call.Note = payload.Note
	}
	m.refreshViewport()
	return m, waitForWSEvent(m.ws.Events())
}

func (m *Model) rebuildUsers() {
	presences := make(map[string]bool, len(m.presence))
	presenceList := make([]models.Presence, 0, len(m.presence))
	for _, p := range m.presence {
		presences[p.Handle] = p.Online
		presenceList = append(presenceList, p)
	}
	m.app.SetUsersDetailed(m.userRoster, presenceList, presences)
}

func (m *Model) refreshViewport() {
	stickToBottom := m.viewport.AtBottom()
	lines := make([]string, 0, len(m.messagesByChannel[m.app.Current])+1)
	for _, msg := range m.messagesByChannel[m.app.Current] {
		lines = append(lines, m.renderMessage(msg))
	}
	var typers []string
	for handle, until := range m.app.Typing {
		if until.After(time.Now()) {
			typers = append(typers, colorHandle(handle).Render(handle))
		}
	}
	if len(typers) > 0 {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(strings.Join(typers, ", ")+" typing..."))
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	if stickToBottom || len(lines) <= m.viewport.Height {
		m.viewport.GotoBottom()
	}
}

func (m *Model) renderMessage(msg models.Message) string {
	contentWidth := max(20, m.viewport.Width-2)
	handleLabel := msg.UserHandle
	if handleLabel == m.app.Handle {
		handleLabel = "you"
	}
	header := fmt.Sprintf("[%s] %s", msg.CreatedAt.Local().Format("15:04"), handleLabel)
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if msg.MessageType == models.MessageTypeSystem {
		body := lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Italic(true).Render(msg.Body)
		return lipgloss.NewStyle().Width(contentWidth).Render(headerStyle.Render(header) + "\n" + body)
	}
	body := msg.Body
	if msg.MessageType == models.MessageTypeEmote {
		if item, ok := emoteByID(msg.Body); ok {
			body = item.Frames[(m.emoteFrame/max(1, int(item.Duration/(180*time.Millisecond))))%len(item.Frames)]
		}
	} else if msg.MessageType == models.MessageTypeMedia && msg.MediaURL != "" {
		url := msg.MediaURL
		if strings.HasPrefix(url, "/") {
			url = strings.TrimRight(m.cfg.ServerURL, "/") + url
		}
		body = msg.Body + "\n" + url
	}
	bodyStyle := lipgloss.NewStyle().Foreground(colorForHandle(msg.UserHandle))
	if msg.MessageType == models.MessageTypeEmote {
		bodyStyle = lipgloss.NewStyle()
	}
	renderedBody := bodyStyle.Render(body)
	if msg.MessageType != models.MessageTypeEmote {
		renderedBody = m.renderStyledBody(msg.UserHandle, body)
	}
	block := headerStyle.Render(header) + "\n" + renderedBody
	if msg.UserHandle == m.app.Handle {
		return lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Right).Render(block)
	}
	return lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Left).Render(block)
}

func (m Model) renderStyledBody(handle, body string) string {
	lines := strings.Split(body, "\n")
	rendered := make([]string, 0, len(lines))
	baseStyle := lipgloss.NewStyle().Foreground(colorForHandle(handle))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "# ") {
			rendered = append(rendered, renderHeadingLine(strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(line, " "), "# ")), baseStyle))
			continue
		}
		rendered = append(rendered, renderStyledLine(line, baseStyle, m.app.Handle))
	}
	return strings.Join(rendered, "\n")
}

func renderStyledLine(line string, baseStyle lipgloss.Style, currentHandle string) string {
	parts := tokenPattern.FindAllString(line, -1)
	currentStyle := baseStyle
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if whitespacePattern.MatchString(part) {
			out.WriteString(part)
			continue
		}
		if styleTag, ok := styleForFormatTag(part, currentStyle); ok {
			currentStyle = styleTag
			continue
		}
		if colorStyle, ok := styleForColorTag(part, currentStyle); ok {
			currentStyle = colorStyle
			continue
		}
		if mention, ok := mentionTarget(part); ok {
			style := currentStyle.
				Underline(true).
				Bold(true).
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("62"))
			if strings.EqualFold(mention, currentHandle) {
				style = style.Background(lipgloss.Color("160"))
			}
			out.WriteString(style.Render(part))
			continue
		}
		if linkText, linkURL, ok := urlToken(part); ok {
			style := currentStyle.Underline(true).Foreground(lipgloss.Color("81"))
			out.WriteString(osc8Link(linkURL, style.Render(linkText)))
			continue
		}
		out.WriteString(currentStyle.Render(part))
	}
	return out.String()
}

func renderHeadingLine(text string, baseStyle lipgloss.Style) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	style := baseStyle.Bold(true)
	rows := make([]string, 5)
	for _, r := range strings.ToUpper(text) {
		glyph, ok := headingBannerFont[r]
		if !ok {
			glyph = []string{
				"███ ",
				fmt.Sprintf(" %c  ", r),
				fmt.Sprintf(" %c  ", r),
				fmt.Sprintf(" %c  ", r),
				"███ ",
			}
		}
		for i := range rows {
			rows[i] += glyph[i] + " "
		}
	}
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
		rows[i] = style.Render(rows[i])
	}
	return strings.Join(rows, "\n")
}

func (m *Model) applySearchLobby() {
	query := strings.ToLower(strings.TrimSpace(m.setupInputs[0].Value()))
	if query == "" || query == "/list" {
		return
	}
	for _, item := range m.savedLobbies {
		if strings.Contains(strings.ToLower(item.LobbyName), query) {
			m.setupInputs[1].SetValue(item.LobbyName)
			m.setupInputs[2].SetValue(item.LobbyCode)
			if item.Handle != "" {
				m.setupInputs[3].SetValue(item.Handle)
			}
			m.app.Notification = "matched remembered lobby"
			return
		}
	}
}

func (m *Model) resizeViewport() {
	leftWidth := min(30, max(24, m.width/4))
	rightWidth := leftWidth
	centerWidth := max(40, m.width-leftWidth-rightWidth-8)
	m.viewport.Width = max(20, centerWidth-4)
	m.viewport.Height = max(8, m.height-19)
}

func (m Model) viewSetup() string {
	style := lipgloss.NewStyle().Padding(1, 2)
	var fields []string
	labels := []string{"Search Lobby (Optional)", "Lobby Name", "Lobby Code", "Username"}
	visibleInputs := 3
	if m.setupStep >= 3 {
		visibleInputs = 4
	}
	for i := 0; i < visibleInputs; i++ {
		input := m.setupInputs[i]
		row := fmt.Sprintf("%s\n%s", labels[i], input.View())
		if i == m.setupStep {
			row = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render(row)
		}
		fields = append(fields, row)
	}
	note := "Press Enter to continue. Type /list in Search Lobby to browse remembered lobbies."
	if remembered, ok := m.lookupRememberedHandle(); ok && remembered.Handle != "" {
		note = "Press Enter on Lobby Code to auto-join. This device already knows your username for this lobby."
	}
	view := style.Render(m.banner() + "\n\n" + strings.Join(fields, "\n\n") + "\n\n" + note)
	if m.showLobbyBrowser {
		view = overlayBox(view, m.lobbyBrowser())
	}
	return view
}

func (m Model) viewChat() string {
	top := lipgloss.NewStyle().Bold(true).Render(m.banner())
	statusBar := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render(
		fmt.Sprintf("status:%s", m.app.StatusText),
	)

	leftWidth := min(30, max(24, m.width/4))
	rightWidth := leftWidth
	centerWidth := max(40, m.width-leftWidth-rightWidth-8)

	leftColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		panelStyle(false, leftWidth).Render("Lobby\n\n"+m.renderLobbyPanel()),
		panelStyle(false, leftWidth).Render("Command Pad\n\n"+m.renderCommandPad()),
	)
	usersBox := panelStyle(m.activePane == 2, rightWidth).Render("Members\n\n" + m.renderUsers())
	messageBox := panelStyle(m.activePane == 1, centerWidth).Render("Messages\n\n" + m.viewport.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, messageBox, usersBox)

	callLine := ""
	if m.app.Call.Status != models.CallStatusIdle {
		callLine = fmt.Sprintf("Call: %s [%s muted=%t] %s\n", m.app.Call.Target, m.app.Call.Status, m.app.Call.Muted, m.app.Call.Note)
	}

	footer := lipgloss.NewStyle().BorderTop(true).Render(
		callLine + "Input\n" + m.input.View() + "\n" + m.app.Notification,
	)

	view := lipgloss.JoinVertical(lipgloss.Left, top, statusBar, body, footer)
	if m.activeEffect != nil {
		if m.activeEffect.Effect == "flash" || m.activeEffect.Effect == "fku" {
			return m.effectOverlay()
		}
		view = overlayBox(view, m.effectOverlay())
	}
	return view
}

func (m Model) renderChannels() string {
	if len(m.app.Channels) == 0 {
		return "no channels yet"
	}
	lines := make([]string, 0, len(m.app.Channels))
	for i, channel := range m.app.Channels {
		line := "#" + channel
		if channel == m.app.Current {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render("> " + line)
		} else if i == m.channelCursor && m.activePane == 0 {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Render("• " + line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderLobbyPanel() string {
	lines := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render("> " + m.app.Workspace),
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderCommandPad() string {
	lines := []string{"/commands --help"}
	if m.commandPadView == "commands" || m.showCommandPanel || m.showHelp {
		lines = append(lines, "")
		lines = append(lines, m.commandGuideLines()...)
	}
	if m.commandPadView == "emotes" || m.showEmotePicker {
		lines = append(lines, "", "Emotes")
		lines = append(lines, m.emotePickerLines()...)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderUsers() string {
	if len(m.app.UserMeta) == 0 {
		return "no members visible"
	}
	lines := make([]string, 0, len(m.app.UserMeta))
	for _, user := range m.app.UserMeta {
		indicator := "○"
		if user.Online {
			indicator = "●"
		}
		line := indicator + " " + colorHandle(user.Handle).Render(user.Handle)
		line += lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("  #" + user.Role)
		if until, ok := m.highlightUntil[user.Handle]; ok && until.After(time.Now()) {
			line = lipgloss.NewStyle().Background(lipgloss.Color("52")).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) helpView() string {
	if m.showCommandPanel {
		return "commands open"
	}
	return ""
}

func (m Model) commandGuideLines() []string {
	return []string{
		"/back",
		"/commands --help",
		"/join <channel>",
		"/dm <username>",
		"/users",
		"/channels",
		"/ping <u|all> [--flash|--fku]",
		"/effects <on|off>",
		"/muteeffects <username>",
		"/chandle <name>",
		"/emote",
		"/me <action>",
		"/clear",
		"/restore --message",
		"/image <path>",
		"/video <path>",
		"/file <path>",
		"/paste",
		"/update",
		"/quit",
	}
}

func (m Model) emotePickerLines() []string {
	pageSize := 10
	if len(emoteCatalog) == 0 {
		return []string{"No emotes loaded"}
	}
	page := (m.emoteCursor / pageSize) + 1
	pageStart := (m.emoteCursor / pageSize) * pageSize
	pageEnd := min(len(emoteCatalog), pageStart+pageSize)
	totalPages := (len(emoteCatalog) + pageSize - 1) / pageSize
	lines := []string{
		fmt.Sprintf("Page %d/%d  Use Up/Down, Left/Right, Enter", page, totalPages),
		"Press 1-9 or 0 for the visible slots",
	}
	for i := pageStart; i < pageEnd; i++ {
		item := emoteCatalog[i]
		slot := ((i - pageStart) + 1) % 10
		if slot == 0 {
			slot = 10
		}
		line := fmt.Sprintf("%d. %s", slot, item.Name)
		if i == m.emoteCursor {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Render("> " + line)
		}
		lines = append(lines, line)
	}
	if len(emoteCatalog) > 0 {
		item := emoteCatalog[m.emoteCursor]
		frame := item.Frames[(m.emoteFrame/max(1, int(item.Duration/(180*time.Millisecond))))%len(item.Frames)]
		lines = append(lines, "", "Preview", frame)
	}
	return lines
}

func (m Model) lobbyBrowser() string {
	lines := []string{"LOBBY LIST", "Use Up/Down + Enter"}
	if len(m.savedLobbies) == 0 {
		lines = append(lines, "", "No saved lobbies yet.")
	} else {
		for i, item := range m.savedLobbies {
			line := fmt.Sprintf("%s  code:%s  username:%s", item.LobbyName, item.LobbyCode, item.Handle)
			if i == m.lobbyCursor {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Render("> " + line)
			}
			lines = append(lines, line)
		}
	}
	return lipgloss.NewStyle().Width(min(76, max(48, m.width/2))).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("81")).Render(strings.Join(lines, "\n"))
}

func (m Model) banner() string {
	if m.width > 0 && m.width < 80 {
		return lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.pixelIcon(),
			"  "+lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Render("TERMICHAT"),
		)
	}
	logo := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render(strings.Join([]string{
		"████████╗███████╗██████╗ ███╗   ███╗██╗ ██████╗██╗  ██╗ █████╗ ████████╗",
		"╚══██╔══╝██╔════╝██╔══██╗████╗ ████║██║██╔════╝██║  ██║██╔══██╗╚══██╔══╝",
		"   ██║   █████╗  ██████╔╝██╔████╔██║██║██║     ███████║███████║   ██║   ",
		"   ██║   ██╔══╝  ██╔══██╗██║╚██╔╝██║██║██║     ██╔══██║██╔══██║   ██║   ",
		"   ██║   ███████╗██║  ██║██║ ╚═╝ ██║██║╚██████╗██║  ██║██║  ██║   ██║   ",
		"   ╚═╝   ╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝╚═╝ ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝   ",
	}, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, m.pixelIcon(), "  ", logo)
}

func (m Model) pixelIcon() string {
	frames := []string{
		"██ ██\n█████\n██ ██\n█████\n██ ██",
		"████ \n██ ██\n█████\n██ ██\n ████",
		"██ ██\n█████\n██ ██\n█████\n██ ██",
		" ████\n██ ██\n█████\n██ ██\n████ ",
	}
	frame := frames[m.emoteFrame%len(frames)]
	return lipgloss.NewStyle().Foreground(lipgloss.Color("87")).Bold(true).Render(frame)
}

func (m *Model) syncChannelCursor() {
	for i, channel := range m.app.Channels {
		if channel == m.app.Current {
			m.channelCursor = i
			return
		}
	}
}

func (m *Model) prefillRememberedHandle() {
	remembered, ok := m.lookupRememberedHandle()
	if !ok || remembered.Handle == "" {
		return
	}
	if strings.TrimSpace(m.setupInputs[3].Value()) == "" || m.setupStep < 3 {
		m.setupInputs[3].SetValue(remembered.Handle)
	}
}

func (m Model) lookupRememberedHandle() (profile.WorkspaceProfile, bool) {
	if m.profiles == nil {
		return profile.WorkspaceProfile{}, false
	}
	serverURL := strings.TrimSpace(m.cfg.ServerURL)
	workspace := strings.TrimSpace(m.setupInputs[1].Value())
	code := strings.TrimSpace(m.setupInputs[2].Value())
	if serverURL == "" || workspace == "" || code == "" {
		return profile.WorkspaceProfile{}, false
	}
	return m.profiles.Lookup(serverURL, workspace, code)
}

func (m Model) shouldAutoConnect() bool {
	return false
}

func waitForWSEvent(ch <-chan clientws.Event) tea.Cmd {
	return func() tea.Msg {
		evt := <-ch
		return wsEventMsg(evt)
	}
}

func panelStyle(active bool, width int) lipgloss.Style {
	border := lipgloss.NormalBorder()
	if active {
		return lipgloss.NewStyle().Width(width).Padding(0, 1).Border(border).BorderForeground(lipgloss.Color("81"))
	}
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Border(border).BorderForeground(lipgloss.Color("240"))
}

func overlayBox(base, overlay string) string {
	return lipgloss.JoinVertical(lipgloss.Left, overlay, base)
}

func (m Model) effectOverlay() string {
	if m.activeEffect == nil {
		return ""
	}
	switch m.activeEffect.Effect {
	case "flash":
		return m.flashOverlay()
	case "fku":
		return m.fkuOverlay()
	default:
		return m.normalPingOverlay()
	}
}

func (m Model) normalPingOverlay() string {
	msg := fmt.Sprintf("PING // %s -> %s", strings.ToUpper(m.activeEffect.From), strings.ToUpper(m.activeEffect.Target))
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("16")).
		Background(lipgloss.Color("221")).
		Bold(true).
		Padding(1, 3).
		Render(msg)
}

func (m Model) flashOverlay() string {
	width := max(20, m.width)
	rows := max(10, m.height)
	progress := m.effectProgress()
	bg := lipgloss.Color("255")
	if progress > 0.45 {
		bg = lipgloss.Color("254")
	}
	if progress > 0.7 {
		bg = lipgloss.Color("252")
	}
	line := strings.Repeat(" ", width)
	block := make([]string, 0, rows)
	jitter := strings.Repeat(" ", m.shakeOffset())
	style := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("16"))
	centerRow := rows / 2
	for i := 0; i < rows; i++ {
		rowText := line
		switch i {
		case centerRow - 1:
			rowText = centerText(width, fmt.Sprintf(" FLASHED BY %s ", strings.ToUpper(m.activeEffect.From)))
		case centerRow:
			rowText = centerText(width, "████████████████████████████")
		case centerRow + 1:
			rowText = centerText(width, fmt.Sprintf(" %0.1fs ", m.activeEffect.EndsAt.Sub(m.activeEffect.StartedAt).Seconds()))
		}
		rowStyle := style
		if i >= centerRow-1 && i <= centerRow+1 {
			rowStyle = rowStyle.Bold(true)
		}
		block = append(block, jitter+rowStyle.Render(padRight(rowText, width)))
	}
	return "\a" + strings.Join(block, "\n")
}

func (m Model) fkuOverlay() string {
	width := max(20, m.width)
	rows := max(10, m.height)
	centerRow := rows / 2
	elapsed := time.Since(m.activeEffect.StartedAt)
	stage := 0
	switch {
	case elapsed >= 1200*time.Millisecond:
		stage = 2
	case elapsed >= 500*time.Millisecond:
		stage = 1
	}

	closedHands := combineHandFrame(pixelHandLeft, pixelHandRight, 4)
	openHands := combineHandFrame(pixelHandLeft, pixelHandRight, 18)
	textFrame := []string{
		"███████╗██╗   ██╗ ██████╗██╗  ██╗",
		"██╔════╝██║   ██║██╔════╝██║ ██╔╝",
		"█████╗  ██║   ██║██║     █████╔╝ ",
		"██╔══╝  ██║   ██║██║     ██╔═██╗ ",
		"██║     ╚██████╔╝╚██████╗██║  ██╗",
		"╚═╝      ╚═════╝  ╚═════╝╚═╝  ╚═╝",
		"",
		fmt.Sprintf("          YOU!!! %s", strings.ToUpper(m.activeEffect.Target)),
	}

	frame := closedHands
	title := fmt.Sprintf("%s is winding up...", strings.ToUpper(m.activeEffect.From))
	if stage == 1 {
		frame = openHands
		title = fmt.Sprintf("%s opened both hands", strings.ToUpper(m.activeEffect.From))
	}
	if stage == 2 {
		frame = textFrame
		title = fmt.Sprintf("%s says:", strings.ToUpper(m.activeEffect.From))
	}

	baseStyle := lipgloss.NewStyle().Background(lipgloss.Color("16")).Foreground(lipgloss.Color("196"))
	block := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		rowText := strings.Repeat(" ", width)
		switch {
		case i == centerRow-4:
			rowText = centerText(width, " TERMINAL ALERT ")
		case i == centerRow-2:
			rowText = centerText(width, title)
		case i >= centerRow && i < centerRow+len(frame):
			rowText = centerANSI(width, frame[i-centerRow])
		}
		rowStyle := baseStyle
		if i == centerRow-4 || i == centerRow-2 || (i >= centerRow && i < centerRow+len(frame)) {
			rowStyle = rowStyle.Bold(true)
		}
		block = append(block, rowStyle.Render(padRight(rowText, width)))
	}
	return "\a" + strings.Join(block, "\n")
}

func (m Model) effectProgress() float64 {
	if m.activeEffect == nil {
		return 1
	}
	total := m.activeEffect.EndsAt.Sub(m.activeEffect.StartedAt)
	if total <= 0 {
		return 1
	}
	elapsed := time.Since(m.activeEffect.StartedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > total {
		elapsed = total
	}
	return float64(elapsed) / float64(total)
}

func (m Model) shakeOffset() int {
	if m.activeEffect == nil || m.activeEffect.Effect != "flash" {
		return 0
	}
	if m.effectProgress() > 0.55 {
		return 0
	}
	if m.emoteFrame%2 == 0 {
		return 2
	}
	return 0
}

func prefixList(prefix string, values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, prefix+v)
	}
	return out
}

var (
	mentionCommandPattern = regexp.MustCompile(`(?i)/mention\s+([a-z0-9._-]+)`)
	tokenPattern          = regexp.MustCompile(`\s+|[^\s]+`)
	whitespacePattern     = regexp.MustCompile(`^\s+$`)
)

func normalizeOutgoingMessage(text string) string {
	return mentionCommandPattern.ReplaceAllString(text, "@$1")
}

func messageMentionsHandle(body, handle string) bool {
	if handle == "" {
		return false
	}
	handle = strings.ToLower(strings.TrimSpace(handle))
	for _, token := range strings.Fields(body) {
		if mention, ok := mentionTarget(token); ok && strings.EqualFold(mention, handle) {
			return true
		}
	}
	return false
}

func mentionTarget(token string) (string, bool) {
	trimmed := strings.TrimSpace(token)
	if !strings.HasPrefix(trimmed, "@") {
		return "", false
	}
	name := strings.TrimPrefix(trimmed, "@")
	name = strings.Trim(name, ".,!?;:()[]{}<>\"'")
	if name == "" {
		return "", false
	}
	return name, true
}

func urlToken(token string) (string, string, bool) {
	trimmed := strings.TrimSpace(token)
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return "", "", false
	}
	urlPart := strings.TrimRight(trimmed, ".,!?;:)]}\"'")
	suffix := strings.TrimPrefix(trimmed, urlPart)
	if urlPart == "" {
		return "", "", false
	}
	return urlPart + suffix, urlPart, true
}

func osc8Link(url, label string) string {
	if url == "" || label == "" {
		return label
	}
	return "\x1b]8;;" + url + "\x1b\\" + label + "\x1b]8;;\x1b\\"
}

func styleForColorTag(token string, currentStyle lipgloss.Style) (lipgloss.Style, bool) {
	colorName := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(token), "#"))
	color, ok := inlineColorMap[colorName]
	if !ok {
		return lipgloss.Style{}, false
	}
	return currentStyle.Foreground(color), true
}

func styleForFormatTag(token string, currentStyle lipgloss.Style) (lipgloss.Style, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "#u":
		return currentStyle.Underline(true), true
	case "#i":
		return currentStyle.Italic(true), true
	default:
		return lipgloss.Style{}, false
	}
}

var inlineColorMap = map[string]lipgloss.Color{
	"red":     lipgloss.Color("196"),
	"orange":  lipgloss.Color("208"),
	"yellow":  lipgloss.Color("226"),
	"green":   lipgloss.Color("46"),
	"cyan":    lipgloss.Color("51"),
	"blue":    lipgloss.Color("39"),
	"purple":  lipgloss.Color("141"),
	"magenta": lipgloss.Color("201"),
	"pink":    lipgloss.Color("213"),
	"white":   lipgloss.Color("255"),
	"gray":    lipgloss.Color("248"),
	"grey":    lipgloss.Color("248"),
}

var headingBannerFont = map[rune][]string{
	' ': {"      ", "      ", "      ", "      ", "      "},
	'!': {" ██   ", " ██   ", " ██   ", "      ", " ██   "},
	'?': {" ████ ", "    ██", "  ███ ", "      ", "  ██  "},
	'.': {"      ", "      ", "      ", "      ", " ██   "},
	',': {"      ", "      ", "      ", "  ██  ", " ██   "},
	'-': {"      ", "      ", " ████ ", "      ", "      "},
	'_': {"      ", "      ", "      ", "      ", "██████"},
	'0': {" ████ ", "██  ██", "██  ██", "██  ██", " ████ "},
	'1': {"  ██  ", "████  ", "  ██  ", "  ██  ", "██████"},
	'2': {"█████ ", "    ██", " ████ ", "██    ", "██████"},
	'3': {"█████ ", "    ██", " ████ ", "    ██", "█████ "},
	'4': {"██  ██", "██  ██", "██████", "    ██", "    ██"},
	'5': {"██████", "██    ", "█████ ", "    ██", "█████ "},
	'6': {" ████ ", "██    ", "█████ ", "██  ██", " ████ "},
	'7': {"██████", "    ██", "   ██ ", "  ██  ", "  ██  "},
	'8': {" ████ ", "██  ██", " ████ ", "██  ██", " ████ "},
	'9': {" ████ ", "██  ██", " █████", "    ██", " ████ "},
	'A': {" ████ ", "██  ██", "██████", "██  ██", "██  ██"},
	'B': {"█████ ", "██  ██", "█████ ", "██  ██", "█████ "},
	'C': {" █████", "██    ", "██    ", "██    ", " █████"},
	'D': {"█████ ", "██  ██", "██  ██", "██  ██", "█████ "},
	'E': {"██████", "██    ", "█████ ", "██    ", "██████"},
	'F': {"██████", "██    ", "█████ ", "██    ", "██    "},
	'G': {" █████", "██    ", "██ ███", "██  ██", " █████"},
	'H': {"██  ██", "██  ██", "██████", "██  ██", "██  ██"},
	'I': {"██████", "  ██  ", "  ██  ", "  ██  ", "██████"},
	'J': {"  ████", "    ██", "    ██", "██  ██", " ████ "},
	'K': {"██  ██", "██ ██ ", "████  ", "██ ██ ", "██  ██"},
	'L': {"██    ", "██    ", "██    ", "██    ", "██████"},
	'M': {"██  ██", "██████", "██ ██ ", "██  ██", "██  ██"},
	'N': {"██  ██", "███ ██", "██ ███", "██  ██", "██  ██"},
	'O': {" ████ ", "██  ██", "██  ██", "██  ██", " ████ "},
	'P': {"█████ ", "██  ██", "█████ ", "██    ", "██    "},
	'Q': {" ████ ", "██  ██", "██  ██", "██ ███", " █████"},
	'R': {"█████ ", "██  ██", "█████ ", "██ ██ ", "██  ██"},
	'S': {" █████", "██    ", " ████ ", "    ██", "█████ "},
	'T': {"██████", "  ██  ", "  ██  ", "  ██  ", "  ██  "},
	'U': {"██  ██", "██  ██", "██  ██", "██  ██", " ████ "},
	'V': {"██  ██", "██  ██", "██  ██", " ████ ", "  ██  "},
	'W': {"██  ██", "██  ██", "██ ██ ", "██████", "██  ██"},
	'X': {"██  ██", " ████ ", "  ██  ", " ████ ", "██  ██"},
	'Y': {"██  ██", " ████ ", "  ██  ", "  ██  ", "  ██  "},
	'Z': {"██████", "   ██ ", "  ██  ", " ██   ", "██████"},
}

func appendMessage(messages []models.Message, msg models.Message) []models.Message {
	messages = append(messages, msg)
	if len(messages) > 500 {
		messages = messages[len(messages)-500:]
	}
	return messages
}

func dmChannelName(a, b string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a > b {
		a, b = b, a
	}
	return "dm-" + a + "-" + b
}

func runSelfUpdate() tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", "irm https://termichat.zeraynce.com/install.ps1 | iex")
		default:
			cmd = exec.Command("sh", "-c", "curl -fsSL https://termichat.zeraynce.com/update.sh | sh")
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := "update failed"
			if len(out) > 0 {
				msg = strings.TrimSpace(string(out))
			}
			return updateResultMsg{Message: msg}
		}
		return updateResultMsg{Message: fmt.Sprintf("updated to %s, restart termichat", buildinfo.Version)}
	}
}

func (m Model) autoUploadInput(text string) (tea.Cmd, string, bool) {
	path, ok := resolveLocalFileReference(text)
	if !ok {
		return nil, "", false
	}
	kind, _, err := detectUploadKindFromPath(path)
	if err != nil {
		return nil, "upload failed: " + err.Error(), true
	}
	return m.runMediaUpload(kind, path), "uploading " + path + "...", true
}

func (m Model) runMediaUpload(kindCmd, path string) tea.Cmd {
	return func() tea.Msg {
		resolvedPath, ok := resolveLocalFileReference(path)
		if !ok {
			resolvedPath = path
		}
		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return updateResultMsg{Message: "upload failed: " + err.Error()}
		}
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(resolvedPath)))
		if contentType == "" && len(data) > 0 {
			contentType = http.DetectContentType(data)
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return m.runMediaUploadData(kindCmd, filepath.Base(resolvedPath), data, contentType)()
	}
}

func (m Model) runClipboardUpload() tea.Cmd {
	return func() tea.Msg {
		fileName, data, contentType, err := readClipboardMedia()
		if err != nil {
			return updateResultMsg{Message: "clipboard upload failed: " + err.Error()}
		}
		kindCmd := detectUploadKindFromContentType(contentType)
		return m.runMediaUploadData(kindCmd, fileName, data, contentType)()
	}
}

func (m Model) runMediaUploadData(kindCmd, fileName string, data []byte, contentType string) tea.Cmd {
	return func() tea.Msg {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("workspace", m.app.Workspace)
		_ = writer.WriteField("code", m.cfg.WorkspaceCode)
		_ = writer.WriteField("handle", m.app.Handle)
		_ = writer.WriteField("device_token", m.deviceToken)
		_ = writer.WriteField("channel", m.app.Current)
		part, err := writer.CreateFormFile("file", fileName)
		if err != nil {
			return updateResultMsg{Message: "upload failed: " + err.Error()}
		}
		if _, err := part.Write(data); err != nil {
			return updateResultMsg{Message: "upload failed: " + err.Error()}
		}
		if err := writer.Close(); err != nil {
			return updateResultMsg{Message: "upload failed: " + err.Error()}
		}
		endpoint := strings.TrimRight(m.cfg.ServerURL, "/") + "/api/media/upload"
		req, err := http.NewRequest(http.MethodPost, endpoint, &body)
		if err != nil {
			return updateResultMsg{Message: "upload failed: " + err.Error()}
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return updateResultMsg{Message: "upload failed: " + err.Error()}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			out, _ := io.ReadAll(resp.Body)
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = resp.Status
			}
			return updateResultMsg{Message: "upload failed: " + msg}
		}
		return updateResultMsg{Message: kindCmd[1:] + " uploaded, chat will update shortly"}
	}
}

func detectUploadKindFromPath(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" && len(data) > 0 {
		contentType = http.DetectContentType(data)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return detectUploadKindFromContentType(contentType), contentType, nil
}

func detectUploadKindFromContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "/image"
	case strings.HasPrefix(contentType, "video/"):
		return "/video"
	default:
		return "/file"
	}
}

func resolveLocalFileReference(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "\"'")
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "file://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false
		}
		raw, _ = url.PathUnescape(u.Path)
	}
	if strings.HasPrefix(raw, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			raw = filepath.Join(home, raw[2:])
		}
	}
	info, err := os.Stat(raw)
	if err != nil || info.IsDir() {
		return "", false
	}
	return raw, true
}

func readClipboardMedia() (string, []byte, string, error) {
	switch runtime.GOOS {
	case "linux":
		return readClipboardMediaLinux()
	case "windows":
		text, err := clipboardTextWindows()
		if err != nil {
			return "", nil, "", err
		}
		path, ok := resolveLocalFileReference(text)
		if !ok {
			return "", nil, "", fmt.Errorf("clipboard does not contain a supported image or file path")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", nil, "", err
		}
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
		if contentType == "" && len(data) > 0 {
			contentType = http.DetectContentType(data)
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return filepath.Base(path), data, contentType, nil
	default:
		return "", nil, "", fmt.Errorf("clipboard media upload is not supported on %s yet", runtime.GOOS)
	}
}

func readClipboardMediaLinux() (string, []byte, string, error) {
	if fileName, data, contentType, ok := readWaylandClipboardImage(); ok {
		return fileName, data, contentType, nil
	}
	if fileName, data, contentType, ok := readXClipClipboardImage(); ok {
		return fileName, data, contentType, nil
	}
	text, err := clipboardTextLinux()
	if err != nil {
		return "", nil, "", err
	}
	path, ok := resolveLocalFileReference(text)
	if !ok {
		return "", nil, "", fmt.Errorf("clipboard does not contain a supported image or file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, "", err
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" && len(data) > 0 {
		contentType = http.DetectContentType(data)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return filepath.Base(path), data, contentType, nil
}

func readWaylandClipboardImage() (string, []byte, string, bool) {
	if _, err := exec.LookPath("wl-paste"); err != nil {
		return "", nil, "", false
	}
	out, err := exec.Command("wl-paste", "--list-types").Output()
	if err != nil {
		return "", nil, "", false
	}
	types := string(out)
	for _, candidate := range []string{"image/png", "image/jpeg", "image/webp", "image/gif"} {
		if !strings.Contains(types, candidate) {
			continue
		}
		data, err := exec.Command("wl-paste", "--no-newline", "--type", candidate).Output()
		if err != nil || len(data) == 0 {
			continue
		}
		return clipboardFileName(candidate), data, candidate, true
	}
	return "", nil, "", false
}

func readXClipClipboardImage() (string, []byte, string, bool) {
	if _, err := exec.LookPath("xclip"); err != nil {
		return "", nil, "", false
	}
	out, err := exec.Command("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o").Output()
	if err != nil {
		return "", nil, "", false
	}
	types := string(out)
	for _, candidate := range []string{"image/png", "image/jpeg", "image/webp", "image/gif"} {
		if !strings.Contains(types, candidate) {
			continue
		}
		data, err := exec.Command("xclip", "-selection", "clipboard", "-t", candidate, "-o").Output()
		if err != nil || len(data) == 0 {
			continue
		}
		return clipboardFileName(candidate), data, candidate, true
	}
	return "", nil, "", false
}

func clipboardTextLinux() (string, error) {
	if _, err := exec.LookPath("wl-paste"); err == nil {
		out, err := exec.Command("wl-paste", "--no-newline").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		out, err := exec.Command("xsel", "--clipboard", "--output").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return "", fmt.Errorf("clipboard tools not found (install wl-clipboard, xclip, or xsel)")
}

func clipboardTextWindows() (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func clipboardFileName(contentType string) string {
	ext := ".bin"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	}
	return fmt.Sprintf("clipboard-%d%s", time.Now().Unix(), ext)
}

func formatUsersSummary(users []state.UserEntry) string {
	if len(users) == 0 {
		return "no users visible"
	}
	parts := make([]string, 0, len(users))
	for _, user := range users {
		status := "offline"
		if user.Online {
			status = "online"
		}
		parts = append(parts, user.Handle+"("+status+")")
	}
	return strings.Join(parts, ", ")
}

func parsePingCommand(fields []string) (string, string, int, error) {
	if len(fields) < 2 {
		return "", "", 0, fmt.Errorf("usage: /ping <handle|all> [--flash|--fku] [-5sc]")
	}
	target := strings.ToLower(strings.TrimSpace(fields[1]))
	effect := "normal"
	durationMS := 0
	for _, field := range fields[2:] {
		switch field {
		case "--flash":
			effect = "flash"
		case "--fku":
			effect = "fku"
		default:
			duration, ok := parsePingDurationFlag(field)
			if !ok {
				return "", "", 0, fmt.Errorf("unknown ping flag %s", field)
			}
			durationMS = duration
		}
	}
	return target, effect, durationMS, nil
}

func parsePingDurationFlag(flag string) (int, bool) {
	raw := strings.TrimSpace(strings.ToLower(flag))
	if raw == "" {
		return 0, false
	}
	raw = strings.TrimPrefix(raw, "-")
	switch {
	case strings.HasSuffix(raw, "sc"):
		raw = strings.TrimSuffix(raw, "sc")
	case strings.HasSuffix(raw, "s"):
		raw = strings.TrimSuffix(raw, "s")
	default:
		return 0, false
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, false
	}
	return seconds * 1000, true
}

func pingNotification(payload protocol.PingReceivedPayload) string {
	target := payload.Target
	if target == "" {
		target = "you"
	}
	if payload.Scope == "all" {
		return fmt.Sprintf("%s pinged everyone (%s)", payload.From, payload.Effect)
	}
	return fmt.Sprintf("%s pinged %s (%s)", payload.From, target, payload.Effect)
}

func centerText(width int, text string) string {
	if width <= len(text) {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text
}

func padRight(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-len(text))
}

func centerANSI(width int, text string) string {
	visible := lipgloss.Width(text)
	if width <= visible {
		return text
	}
	padding := (width - visible) / 2
	return strings.Repeat(" ", padding) + text
}

func combineHandFrame(left, right string, gap int) []string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	size := max(len(leftLines), len(rightLines))
	lines := make([]string, 0, size)
	for i := 0; i < size; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lines = append(lines, l+strings.Repeat(" ", gap)+r)
	}
	return lines
}

func colorHandle(handle string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorForHandle(handle)).Bold(true)
}

func colorForHandle(handle string) lipgloss.Color {
	palette := []lipgloss.Color{"81", "117", "159", "186", "221", "214", "177", "148", "121", "45"}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(handle)))
	return palette[int(h.Sum32())%len(palette)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
