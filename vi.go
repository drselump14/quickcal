package main

// Minimal vi editing for the single-line input, with undo/redo.
//
// normal: h l w b e W B E 0 ^ $ f F t T ; ,  (counts: 3w, 2dw)
//         i a I A  x X r ~ D C s S  p P  u ctrl+r
//         d c y + motion, dd cc yy (whole line)

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type viMode int

const (
	insertMode viMode = iota
	normalMode
)

type snap struct {
	val string
	pos int
}

type vi struct {
	mode        viMode
	pending     []rune // waiting operator / char: "d", "f", "df", "r", "2d"…
	count       int
	reg         string // unnamed register
	undo, redo  []snap
	insertStart snap // state when the current insert session began
	lastFind    [2]rune
}

func cur(ti *textinput.Model) snap { return snap{ti.Value(), ti.Position()} }

func set(ti *textinput.Model, s snap) {
	ti.SetValue(s.val)
	ti.SetCursor(s.pos)
}

// save records the state before a change (clears redo).
func (v *vi) save(s snap) {
	if n := len(v.undo); n > 0 && v.undo[n-1] == s {
		return
	}
	v.undo = append(v.undo, s)
	if len(v.undo) > 200 {
		v.undo = v.undo[1:]
	}
	v.redo = nil
}

// commitInsert turns the running insert session into one undo step.
func (v *vi) commitInsert(ti *textinput.Model) {
	if ti.Value() != v.insertStart.val {
		v.save(v.insertStart)
	}
	v.insertStart = cur(ti)
}

func (v *vi) Undo(ti *textinput.Model) bool {
	if v.mode == insertMode {
		v.commitInsert(ti)
	}
	if len(v.undo) == 0 {
		return false
	}
	v.redo = append(v.redo, cur(ti))
	s := v.undo[len(v.undo)-1]
	v.undo = v.undo[:len(v.undo)-1]
	set(ti, s)
	v.insertStart = s
	return true
}

func (v *vi) Redo(ti *textinput.Model) bool {
	if len(v.redo) == 0 {
		return false
	}
	v.undo = append(v.undo, cur(ti))
	s := v.redo[len(v.redo)-1]
	v.redo = v.redo[:len(v.redo)-1]
	set(ti, s)
	v.insertStart = s
	return true
}

// Clear empties the input as an undoable step (used after adding an event).
func (v *vi) Clear(ti *textinput.Model) {
	if v.mode == insertMode {
		v.commitInsert(ti)
	}
	if ti.Value() != "" {
		v.save(cur(ti))
	}
	ti.SetValue("")
	v.insertStart = cur(ti)
}

func (v *vi) EnterInsert(ti *textinput.Model, pos int) {
	v.mode = insertMode
	ti.SetCursor(pos)
	v.insertStart = cur(ti)
}

func (v *vi) EnterNormal(ti *textinput.Model) {
	v.commitInsert(ti)
	v.mode = normalMode
	v.pending, v.count = nil, 0
	ti.SetCursor(max(ti.Position()-1, 0))
}

// ------------------------------------------------------------------ words

func cls(r rune, big bool) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case big || unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	default:
		return 2
	}
}

func nextWordStart(rs []rune, i int, big bool) int {
	n := len(rs)
	if i >= n {
		return n
	}
	if c := cls(rs[i], big); c != 0 {
		for i < n && cls(rs[i], big) == c {
			i++
		}
	}
	for i < n && cls(rs[i], big) == 0 {
		i++
	}
	return i
}

func prevWordStart(rs []rune, i int, big bool) int {
	if i <= 0 {
		return 0
	}
	i--
	for i > 0 && cls(rs[i], big) == 0 {
		i--
	}
	c := cls(rs[i], big)
	for i > 0 && cls(rs[i-1], big) == c {
		i--
	}
	return i
}

func wordEnd(rs []rune, i int, big bool) int {
	n := len(rs)
	if i >= n-1 {
		return max(n-1, 0)
	}
	i++
	for i < n && cls(rs[i], big) == 0 {
		i++
	}
	if i >= n {
		return n - 1
	}
	c := cls(rs[i], big)
	for i < n-1 && cls(rs[i+1], big) == c {
		i++
	}
	return i
}

func firstNonBlank(rs []rune) int {
	for i, r := range rs {
		if !unicode.IsSpace(r) {
			return i
		}
	}
	return 0
}

func find(rs []rune, pos int, cmd, ch rune, count int) (int, bool) {
	i := pos
	for range count {
		switch cmd {
		case 'f', 't':
			j := i + 1
			if cmd == 't' {
				j = i + 2 // skip the char we're already before
			}
			k := indexFrom(rs, ch, j)
			if k < 0 {
				return pos, false
			}
			i = k
			if cmd == 't' {
				i = k - 1
			}
		case 'F', 'T':
			j := i - 1
			if cmd == 'T' {
				j = i - 2
			}
			k := lastIndexBefore(rs, ch, j)
			if k < 0 {
				return pos, false
			}
			i = k
			if cmd == 'T' {
				i = k + 1
			}
		}
	}
	return i, true
}

func indexFrom(rs []rune, ch rune, from int) int {
	for k := max(from, 0); k < len(rs); k++ {
		if rs[k] == ch {
			return k
		}
	}
	return -1
}

func lastIndexBefore(rs []rune, ch rune, from int) int {
	for k := min(from, len(rs)-1); k >= 0; k-- {
		if rs[k] == ch {
			return k
		}
	}
	return -1
}

// motion returns the target and whether the range end is inclusive.
func motion(rs []rune, pos int, m rune, count int) (int, bool, bool) {
	n := len(rs)
	t := pos
	switch m {
	case 'h':
		return max(pos-count, 0), false, true
	case 'l', ' ':
		return min(pos+count, n), false, true
	case '0':
		return 0, false, true
	case '^':
		return firstNonBlank(rs), false, true
	case '$':
		return n, false, true
	case 'w', 'W':
		for range count {
			t = nextWordStart(rs, t, m == 'W')
		}
		return t, false, true
	case 'b', 'B':
		for range count {
			t = prevWordStart(rs, t, m == 'B')
		}
		return t, false, true
	case 'e', 'E':
		for range count {
			t = wordEnd(rs, t, m == 'E')
		}
		return t, true, true
	}
	return pos, false, false
}

// ------------------------------------------------------------------ normal

// Normal handles a key in normal mode; returns true when the text changed.
func (v *vi) Normal(msg tea.KeyMsg, ti *textinput.Model) bool {
	var ch rune
	switch {
	case msg.Type == tea.KeyRunes && len(msg.Runes) == 1:
		ch = msg.Runes[0]
	case msg.Type == tea.KeySpace:
		ch = ' '
	case msg.String() == "ctrl+r":
		v.pending, v.count = nil, 0
		return v.Redo(ti)
	case msg.Type == tea.KeyEsc:
		v.pending, v.count = nil, 0
		return false
	case msg.Type == tea.KeyLeft:
		ch = 'h'
	case msg.Type == tea.KeyRight:
		ch = 'l'
	case msg.Type == tea.KeyBackspace:
		ch = 'h'
	default:
		return false
	}
	rs := []rune(ti.Value())
	pos := min(ti.Position(), len(rs))
	n := len(rs)

	// waiting for a character argument (r, f/t, or operator + f/t)
	if len(v.pending) > 0 {
		last := v.pending[len(v.pending)-1]
		switch last {
		case 'r':
			v.pending = nil
			if pos < n {
				v.save(cur(ti))
				rs[pos] = ch
				ti.SetValue(string(rs))
				ti.SetCursor(pos)
				return true
			}
			return false
		case 'f', 'F', 't', 'T':
			op := v.pending[:len(v.pending)-1]
			v.pending = nil
			v.lastFind = [2]rune{last, ch}
			t, ok := find(rs, pos, last, ch, v.takeCount())
			if !ok {
				return false
			}
			if len(op) == 0 {
				ti.SetCursor(t)
				return false
			}
			return v.operate(ti, op[0], pos, t, last == 'f' || last == 't')
		}
	}

	// counts
	if ch >= '1' && ch <= '9' || ch == '0' && v.count > 0 {
		v.count = v.count*10 + int(ch-'0')
		return false
	}

	// operator pending: d / c / y
	if len(v.pending) == 1 && strings.ContainsRune("dcy", v.pending[0]) {
		op := v.pending[0]
		switch {
		case ch == op: // dd cc yy
			v.pending = nil
			v.count = 0
			return v.operate(ti, op, 0, n, false)
		case strings.ContainsRune("fFtT", ch):
			v.pending = append(v.pending, ch)
			return false
		}
		v.pending = nil
		count := v.takeCount()
		if op == 'c' && (ch == 'w' || ch == 'W') && pos < n && !unicode.IsSpace(rs[pos]) {
			ch = map[rune]rune{'w': 'e', 'W': 'E'}[ch] // vim: cw acts like ce
		}
		t, incl, ok := motion(rs, pos, ch, count)
		if !ok {
			return false
		}
		return v.operate(ti, op, pos, t, incl)
	}

	count := v.takeCount()
	switch ch {
	case 'i':
		v.EnterInsert(ti, pos)
	case 'a':
		v.EnterInsert(ti, min(pos+1, n))
	case 'I':
		v.EnterInsert(ti, firstNonBlank(rs))
	case 'A':
		v.EnterInsert(ti, n)
	case 'd', 'c', 'y':
		v.pending = []rune{ch}
		v.count = count
		if count == 1 {
			v.count = 0
		}
	case 'r', 'f', 'F', 't', 'T':
		v.pending = []rune{ch}
		v.count = count
		if count == 1 {
			v.count = 0
		}
	case ';', ',':
		if v.lastFind[0] == 0 {
			return false
		}
		cmd := v.lastFind[0]
		if ch == ',' {
			cmd = map[rune]rune{'f': 'F', 'F': 'f', 't': 'T', 'T': 't'}[cmd]
		}
		if t, ok := find(rs, pos, cmd, v.lastFind[1], count); ok {
			ti.SetCursor(t)
		}
	case 'x':
		if n > 0 {
			return v.operate(ti, 'd', pos, min(pos+count, n), false)
		}
	case 'X':
		if pos > 0 {
			return v.operate(ti, 'd', max(pos-count, 0), pos, false)
		}
	case 's':
		return v.operate(ti, 'c', pos, min(pos+count, n), false)
	case 'S':
		return v.operate(ti, 'c', 0, n, false)
	case 'D':
		return v.operate(ti, 'd', pos, n, false)
	case 'C':
		return v.operate(ti, 'c', pos, n, false)
	case '~':
		if pos < n {
			v.save(cur(ti))
			for k := pos; k < min(pos+count, n); k++ {
				if unicode.IsUpper(rs[k]) {
					rs[k] = unicode.ToLower(rs[k])
				} else {
					rs[k] = unicode.ToUpper(rs[k])
				}
			}
			ti.SetValue(string(rs))
			ti.SetCursor(min(pos+count, n-1))
			return true
		}
	case 'p', 'P':
		if v.reg == "" {
			return false
		}
		at := pos
		if ch == 'p' && n > 0 {
			at = pos + 1
		}
		v.save(cur(ti))
		ins := []rune(strings.Repeat(v.reg, count))
		out := append(append(append([]rune{}, rs[:at]...), ins...), rs[at:]...)
		ti.SetValue(string(out))
		ti.SetCursor(at + len(ins) - 1)
		return true
	case 'u':
		changed := false
		for range count {
			changed = v.Undo(ti) || changed
		}
		v.clamp(ti)
		return changed
	default:
		if t, _, ok := motion(rs, pos, ch, count); ok {
			ti.SetCursor(t)
			v.clamp(ti)
		}
	}
	return false
}

func (v *vi) takeCount() int {
	c := max(v.count, 1)
	v.count = 0
	return c
}

// clamp keeps the normal-mode cursor on a character (not past the end).
func (v *vi) clamp(ti *textinput.Model) {
	if v.mode == normalMode {
		if n := len([]rune(ti.Value())); ti.Position() >= n {
			ti.SetCursor(max(n-1, 0))
		}
	}
}

// operate applies d/c/y over [from,to) (to included when incl).
func (v *vi) operate(ti *textinput.Model, op rune, from, to int, incl bool) bool {
	rs := []rune(ti.Value())
	a, b := min(from, to), max(from, to)
	if incl {
		b++
	}
	b = min(b, len(rs))
	if a >= b && op != 'c' {
		return false
	}
	v.reg = string(rs[a:b])
	if op == 'y' {
		ti.SetCursor(a)
		return false
	}
	v.save(cur(ti))
	ti.SetValue(string(rs[:a]) + string(rs[b:]))
	if op == 'c' {
		v.EnterInsert(ti, a)
		v.insertStart = cur(ti) // the delete above is already its own undo step
	} else {
		ti.SetCursor(a)
		v.clamp(ti)
	}
	return true
}
