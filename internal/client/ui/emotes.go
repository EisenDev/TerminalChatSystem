package ui

import "time"

type emote struct {
	ID       string
	Number   int
	Name     string
	Frames   []string
	Duration time.Duration
}

var emoteCatalog = []emote{
	{ID: "happy", Number: 1, Name: "happy", Frames: []string{"[ ^_^ ]", "[ ^o^ ]"}, Duration: 220 * time.Millisecond},
	{ID: "angry", Number: 2, Name: "angry", Frames: []string{"[ >:( ]", "[ >:| ]"}, Duration: 220 * time.Millisecond},
	{ID: "laugh", Number: 3, Name: "laugh", Frames: []string{"[ xD ]", "[ XD ]"}, Duration: 180 * time.Millisecond},
	{ID: "cry", Number: 4, Name: "cry", Frames: []string{"[ ;_; ]", "[ T_T ]"}, Duration: 240 * time.Millisecond},
	{ID: "wave", Number: 5, Name: "wave", Frames: []string{"o/ TEAM", "\\o TEAM"}, Duration: 180 * time.Millisecond},
	{ID: "thumbsup", Number: 6, Name: "thumbsup", Frames: []string{"[==b]", "[= b]"}, Duration: 220 * time.Millisecond},
	{ID: "dance", Number: 7, Name: "dance", Frames: []string{"<o/  ", " \\o> ", "<o>  "}, Duration: 160 * time.Millisecond},
	{ID: "clap", Number: 8, Name: "clap", Frames: []string{"o/ \\o", "o\\ /o"}, Duration: 160 * time.Millisecond},
	{ID: "fire", Number: 9, Name: "fire", Frames: []string{" .^. ", " /#\\ ", " .^. "}, Duration: 150 * time.Millisecond},
	{ID: "salute", Number: 10, Name: "salute", Frames: []string{"[ o7 ]", "[ -7 ]"}, Duration: 220 * time.Millisecond},
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
