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
	{
		ID:     "catwave",
		Number: 1,
		Name:   "cat wave",
		Frames: []string{
			" /\\_/\\\\\n( o.o )\n / ^\\\\ ",
			" /\\_/\\\\\n( o.o )\n /o^\\\\ ",
		},
		Duration: 220 * time.Millisecond,
	},
	{
		ID:     "monkeycry",
		Number: 2,
		Name:   "monkey cry",
		Frames: []string{
			"  .--.  \n ( ;_; )\n /|   |\\\\",
			"  .--.  \n ( T_T )\n /|   |\\\\",
		},
		Duration: 240 * time.Millisecond,
	},
	{
		ID:     "slimedance",
		Number: 3,
		Name:   "slime dance",
		Frames: []string{
			"  ____  \n / oo \\\\\n \\____/",
			"  ____  \n / oo \\\\\n_/____\\_",
			" ______ \n/ oo  \\\\\n\\_____/ ",
		},
		Duration: 170 * time.Millisecond,
	},
	{
		ID:     "foxangry",
		Number: 4,
		Name:   "fox angry",
		Frames: []string{
			" /\\   /\\\\\n( >_< )\n \\/___\\/ ",
			" /\\   /\\\\\n( >:c )\n \\/___\\/ ",
		},
		Duration: 220 * time.Millisecond,
	},
	{
		ID:     "owlblink",
		Number: 5,
		Name:   "owl blink",
		Frames: []string{
			" ,_, \n( o.o )\n /)_) ",
			" ,_, \n( -.- )\n /)_) ",
		},
		Duration: 260 * time.Millisecond,
	},
	{
		ID:     "dogcheer",
		Number: 6,
		Name:   "dog cheer",
		Frames: []string{
			" / \\__\n(    @\\___\n /         O",
			" / \\__\n(    ^\\___\n /         O",
		},
		Duration: 220 * time.Millisecond,
	},
	{
		ID:     "frogwow",
		Number: 7,
		Name:   "frog wow",
		Frames: []string{
			"  @..@ \n (----)\n( >__< )",
			"  @..@ \n (----)\n( 0__0 )",
		},
		Duration: 230 * time.Millisecond,
	},
	{
		ID:     "bunnyjump",
		Number: 8,
		Name:   "bunny jump",
		Frames: []string{
			" (\\_/)\n (o.o)\n /|_|\\\\",
			" (\\_/)\n (o.o)\n _/|_|\\\\_",
		},
		Duration: 180 * time.Millisecond,
	},
	{
		ID:     "penguinsalute",
		Number: 9,
		Name:   "penguin salute",
		Frames: []string{
			"  _~_  \n (o o)\n / V \\\\",
			"  _~_  \n (o o)\n / V7\\\\",
		},
		Duration: 220 * time.Millisecond,
	},
	{
		ID:     "ghostboo",
		Number: 10,
		Name:   "ghost boo",
		Frames: []string{
			" .-. \n(o o)\n| O \\\n|   \\\n'~~~'",
			" .-. \n(O O)\n| O \\\n|   \\\n'~~~'",
		},
		Duration: 210 * time.Millisecond,
	},
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
