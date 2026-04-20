package call

import "github.com/eisen/teamchat/internal/shared/models"

type State struct {
	Target string
	Status models.CallStatus
	Muted  bool
	Note   string
}

func InitialState() State {
	return State{Status: models.CallStatusIdle}
}
