package main

import (
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eisen/teamchat/internal/client/ui"
	"github.com/eisen/teamchat/internal/shared/config"
	"github.com/eisen/teamchat/internal/shared/logging"
)

func main() {
	cfg := config.LoadClient()
	logger := logging.New(cfg.LogFormat, slog.LevelInfo)

	program := tea.NewProgram(
		ui.NewModel(cfg, logger),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := program.Run(); err != nil {
		logger.Error("client exited with error", "error", err)
		os.Exit(1)
	}
}
