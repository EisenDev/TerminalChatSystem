package ui

import "time"

type emote struct {
	ID       string
	Number   int
	Name     string
	Frames   []string
	Duration time.Duration
}

func emoteByID(id string) (emote, bool) {
	for _, item := range emoteCatalog {
		if item.ID == id {
			return item, true
		}
	}
	return emote{}, false
}

func emoteByNumber(n int) (emote, bool) {
	for _, item := range emoteCatalog {
		if item.Number == n {
			return item, true
		}
	}
	return emote{}, false
}
