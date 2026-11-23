package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// feed sends normal-mode keys; "<cr>" style tokens aren't needed here.
func feed(v *vi, ti *textinput.Model, keys string) {
	for _, r := range keys {
		if v.mode == insertMode { // typed text in insert mode
			if r == '\x1b' {
				v.EnterNormal(ti)
				continue
			}
			rs := []rune(ti.Value())
			p := ti.Position()
			ti.SetValue(string(rs[:p]) + string(r) + string(rs[p:]))
			ti.SetCursor(p + 1)
			continue
		}
		if r == '\x12' { // ctrl+r
			v.Normal(tea.KeyMsg{Type: tea.KeyCtrlR}, ti)
			continue
		}
		v.Normal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, ti)
	}
}

func setup(text string) (*vi, *textinput.Model) {
	ti := textinput.New()
	ti.SetValue(text)
	v := &vi{mode: normalMode}
	ti.SetCursor(0)
	return v, &ti
}

func TestViEditing(t *testing.T) {
	cases := []struct{ start, keys, want string }{
		{"Lunch tomorrow 3pm", "dw", "tomorrow 3pm"},
		{"Lunch tomorrow 3pm", "$x", "Lunch tomorrow 3p"},
		{"Lunch tomorrow 3pm", "wcwtoday\x1b", "Lunch today 3pm"},
		{"Lunch tomorrow 3pm", "2dw", "3pm"},
		{"Lunch tomorrow 3pm", "dd", ""},
		{"Lunch tomorrow 3pm", "ft;D", "Lunch "}, // ; finds no second t, cursor stays
		{"Lunch tomorrow 3pm", "dt ", " tomorrow 3pm"},
		{"Lunch tomorrow 3pm", "ywP", "Lunch Lunch tomorrow 3pm"},
		{"lunch", "~", "Lunch"},
		{"Lunch at 3pm", "fxrX", "Xunch at 3pm"}, // failed fx is ignored, rX applies
		{"Lunch at 3pm", "$bcwnoon\x1b", "Lunch at noon"},
		{"Lunch at 3pm", "A /work\x1b", "Lunch at 3pm /work"},
		{"明日15時 歯医者", "wD", "明日15時 "},
	}
	for _, c := range cases {
		v, ti := setup(c.start)
		feed(v, ti, c.keys)
		if got := ti.Value(); got != c.want {
			t.Errorf("%q + %q = %q, want %q", c.start, c.keys, got, c.want)
		}
	}
}

func TestViUndoRedo(t *testing.T) {
	v, ti := setup("Lunch tomorrow 3pm")
	feed(v, ti, "dwdw")
	if ti.Value() != "3pm" {
		t.Fatalf("got %q", ti.Value())
	}
	feed(v, ti, "u")
	if ti.Value() != "tomorrow 3pm" {
		t.Fatalf("undo1: %q", ti.Value())
	}
	feed(v, ti, "u")
	if ti.Value() != "Lunch tomorrow 3pm" {
		t.Fatalf("undo2: %q", ti.Value())
	}
	feed(v, ti, "\x12")
	if ti.Value() != "tomorrow 3pm" {
		t.Fatalf("redo: %q", ti.Value())
	}
	// an insert session is one undo step
	feed(v, ti, "A extra words\x1b")
	feed(v, ti, "u")
	if ti.Value() != "tomorrow 3pm" {
		t.Fatalf("insert undo: %q", ti.Value())
	}
}

func TestClearRestores(t *testing.T) {
	// what happens after adding an event: input cleared, `esc u` restores
	ti := textinput.New()
	v := &vi{mode: insertMode}
	ti.SetValue("Dentist fri 3pm")
	v.Clear(&ti)
	if ti.Value() != "" {
		t.Fatal("not cleared")
	}
	v.EnterNormal(&ti)
	feed(v, &ti, "u")
	if ti.Value() != "Dentist fri 3pm" {
		t.Fatalf("restore: %q", ti.Value())
	}
}
