package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/drselump14/quickcal/parse"
)

// Editor mode, used by calendar front-ends (e.g. qkal) to edit an existing
// event with quickcal's UI:
//
//	quickcal --edit "Lunch 2026-11-27 12:00-13:00 @ Sushiro /work" --edit-out /tmp/x.json
//
// The input starts with TEXT; enter writes the parsed event (parse.Event as
// JSON, including any AI fallback result) to FILE and quits. Nothing is
// written to khal; the caller applies the change. Quitting with ^q / ^c
// leaves FILE untouched, which means "cancelled".

var (
	editText string
	editOut  string
	edited   *parse.Event // the event as it was, to leave it out of conflicts
)

// parseEditFlags removes --edit TEXT / --edit-out FILE from os.Args.
func parseEditFlags() error {
	args := []string{os.Args[0]}
	for i := 1; i < len(os.Args); i++ {
		switch a := os.Args[i]; {
		case a == "--edit" || a == "--edit-out":
			if i+1 >= len(os.Args) {
				return errors.New(a + " needs a value")
			}
			if a == "--edit" {
				editText = os.Args[i+1]
			} else {
				editOut = os.Args[i+1]
			}
			i++
		case strings.HasPrefix(a, "--edit="):
			editText = strings.TrimPrefix(a, "--edit=")
		case strings.HasPrefix(a, "--edit-out="):
			editOut = strings.TrimPrefix(a, "--edit-out=")
		default:
			args = append(args, a)
		}
	}
	os.Args = args
	if (editText == "") != (editOut == "") {
		return errors.New("--edit and --edit-out go together")
	}
	return nil
}

func (m *model) startEdit(text string) {
	m.input.SetValue(text)
	m.input.CursorStart() // title first; long notes run off to the right
	m.ev = parse.Parse(text, m.cals, m.curCal, time.Now())
	m.focus = m.ev.StartDate
	orig := *m.ev
	edited = &orig
}

func writeEdit(ev *parse.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	tmp := editOut + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, editOut)
}

// withoutEdited drops the event being edited from a conflict list.
func withoutEdited(conf []kEvent) []kEvent {
	if edited == nil {
		return conf
	}
	s, ok := edited.Start()
	if !ok {
		return conf
	}
	var out []kEvent
	for _, k := range conf {
		// same start and the parsed title inside the real one ("Weekly sync"
		// parses as title "sync" + weekly repeat)
		if k.Start.Equal(s) && strings.Contains(k.Title, edited.Title) {
			continue
		}
		out = append(out, k)
	}
	return out
}
