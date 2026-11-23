// quickcal - natural-language event entry for khal.
//
//	quickcal                      TUI
//	quickcal -n "gym mon 7pm"     print the khal command (dry run)
//	quickcal -x "gym mon 7pm"     create the event directly
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/drselump14/quickcal/parse"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// content width: one compact column
func (m model) cw() int { return min(max(m.w-2, 46), 100) }

var (
	dim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	green   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	cyan    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	magenta = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	yellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	red     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	bold    = lipgloss.NewStyle().Bold(true)

	box = func(c string) lipgloss.Style {
		return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(c)).Padding(0, 1)
	}
)

// ------------------------------------------------------------------ messages

type dayMsg struct {
	day string
	evs []kEvent
	err error
}
type aiTickMsg struct {
	seq  int
	text string
}
type aiMsg struct {
	text string
	res  *aiResult
	err  error
}

func runAI(text string) tea.Cmd {
	return func() tea.Msg {
		r, err := aiParse(text, time.Now())
		return aiMsg{text, r, err}
	}
}

type createdMsg struct {
	ev  *parse.Event
	err error
}

func loadDayCmd(d time.Time) tea.Cmd {
	return func() tea.Msg {
		evs, err := fetchDay(d)
		return dayMsg{d.Format("2006-01-02"), evs, err}
	}
}
func createCmd(ev *parse.Event) tea.Cmd {
	return func() tea.Msg { return createdMsg{ev, createEvent(ev.KhalArgs())} }
}

// ------------------------------------------------------------------ model

type model struct {
	input    textinput.Model
	cals     []string
	writable []string
	curCal   string
	ev       *parse.Event
	focus    time.Time // date shown in month/day panes
	dayCache map[string][]kEvent
	loading  map[string]bool
	status   string
	ikhalID  string // kitty window id of our ikhal pane
	aiOn     bool
	aiSeq    int                  // debounce generation
	aiState  string               // "", "waiting", "asking", "done", or an error
	aiCache  map[string]*aiResult // keyed by aiInput(text)
	confirm  bool                 // enter pressed while AI still running
	vi       vi
	w, h     int
}

// fit truncates every line to width w (ANSI- and CJK-aware).
func fit(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	return strings.Join(lines, "\n")
}

func newModel() model {
	cals, def := loadCalendars()
	var writable []string
	for _, c := range cals {
		if !strings.Contains(c, "holiday") {
			writable = append(writable, c)
		}
	}
	ti := textinput.New()
	ti.Placeholder = "Lunch with Ana at Sushiro tomorrow 12:30 for 90m /work"
	ti.Prompt = "❯ "
	ti.PromptStyle = green
	ti.Focus()
	return model{
		input: ti,
		cals:  cals, writable: writable, curCal: def,
		focus: today(), dayCache: map[string][]kEvent{}, loading: map[string]bool{},
		aiOn: aiEnabled(), aiCache: map[string]*aiResult{},
	}
}

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.ensureDays(m.focus), fitWindow(false))
}

// ensureDays starts khal fetches for days not cached or in flight.
func (m *model) ensureDays(days ...time.Time) tea.Cmd {
	var cmds []tea.Cmd
	for _, d := range days {
		k := d.Format("2006-01-02")
		if _, ok := m.dayCache[k]; ok || m.loading[k] {
			continue
		}
		m.loading[k] = true
		cmds = append(cmds, loadDayCmd(d))
	}
	return tea.Batch(cmds...)
}

func (m *model) resetDays() {
	m.dayCache = map[string][]kEvent{}
	m.loading = map[string]bool{}
}

func (m *model) reparse() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		m.ev = nil
		return m.setFocus(today())
	}
	m.ev = parse.Parse(text, m.cals, m.curCal, time.Now())
	m.aiState, m.confirm = "", false
	aiCmd := m.maybeAI(false)
	return tea.Batch(aiCmd, m.setFocus(m.ev.StartDate), m.ensureDays(spanDays(m.ev)...))
}

// maybeAI applies a cached AI result, or schedules aichat when needed.
func (m *model) maybeAI(force bool) tea.Cmd {
	if !m.aiOn || m.ev == nil {
		return nil
	}
	raw := m.input.Value()
	if !force && !needsAI(raw, m.ev) {
		return nil
	}
	key := aiInput(raw)
	if key == "" {
		return nil
	}
	if r, ok := m.aiCache[key]; ok {
		m.useAI(r)
		return nil
	}
	m.aiSeq++
	if force {
		m.aiState = "asking"
		return runAI(key)
	}
	m.aiState = "waiting"
	seq := m.aiSeq
	return tea.Tick(aiDebounce, func(time.Time) tea.Msg { return aiTickMsg{seq, key} })
}

func (m *model) useAI(r *aiResult) {
	ev, err := applyAI(m.ev, r)
	if err != nil {
		m.aiState = err.Error()
		return
	}
	m.ev, m.aiState = ev, "done"
}

// reload drops cached days and refetches the ones on screen.
func (m *model) reload() tea.Cmd {
	m.resetDays()
	days := []time.Time{m.focus}
	if m.ev != nil {
		days = append(days, spanDays(m.ev)...)
	}
	return m.ensureDays(days...)
}

func (m *model) setFocus(d time.Time) tea.Cmd {
	m.focus = d
	return m.ensureDays(d)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.input.Width = m.cw() - 8
		return m, nil

	case tea.FocusMsg: // back from ikhal etc.: events may have changed
		return m, tea.Batch(m.reload(), ikhalAlive(m.ikhalID))

	case aiTickMsg:
		if msg.seq != m.aiSeq || m.aiState != "waiting" {
			return m, nil // typing continued; a newer tick is coming
		}
		m.aiState = "asking"
		return m, runAI(msg.text)

	case aiMsg:
		current := m.ev != nil && aiInput(m.input.Value()) == msg.text
		if msg.err != nil {
			if current {
				m.aiState = msg.err.Error()
			}
			return m, nil
		}
		m.aiCache[msg.text] = msg.res
		if !current {
			return m, nil
		}
		m.useAI(msg.res)
		return m, tea.Batch(m.setFocus(m.ev.StartDate), m.ensureDays(spanDays(m.ev)...))

	case ikhalMsg:
		switch {
		case msg.err != nil:
			m.status = red.Render(msg.err.Error())
		case msg.closed:
			m.ikhalID = ""
			m.status = dim.Render("ikhal closed")
			return m, tea.Batch(m.reload(), fitWindow(false))
		default:
			m.ikhalID = msg.id
			m.status = dim.Render("ikhal opened · ctrl+o again to close")
		}
		return m, nil

	case ikhalDoneMsg:
		if msg.err != nil {
			m.status = red.Render("ikhal: " + msg.err.Error())
		}
		return m, m.reload()

	case dayMsg:
		delete(m.loading, msg.day)
		if msg.err == nil {
			m.dayCache[msg.day] = msg.evs
		} else {
			m.status = red.Render("khal list failed: " + msg.err.Error())
		}
		return m, nil

	case createdMsg:
		if msg.err != nil {
			m.status = red.Bold(true).Render("✗ khal failed: " + msg.err.Error())
			return m, nil
		}
		ev := msg.ev
		when := ev.StartDate.Format("2006-01-02")
		if s, ok := ev.Start(); ok {
			when += s.Format(" 15:04")
		}
		m.status = green.Bold(true).Render(fmt.Sprintf("✓ added '%s' · %s · %s", ev.Title, when, ev.Calendar))
		m.resetDays()
		m.vi.Clear(&m.input) // undoable: esc u brings the text back
		m.ev = nil
		return m, m.ensureDays(ev.StartDate)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		case "enter":
			switch {
			case m.ev == nil:
				m.status = yellow.Render("nothing to add")
			case m.ev.Title == "":
				m.status = red.Render("need a title")
			case (m.aiState == "waiting" || m.aiState == "asking") && !m.confirm:
				m.confirm = true
				m.status = yellow.Render("aichat still parsing… enter again to add as shown")
			default:
				m.status = dim.Render("adding…")
				return m, createCmd(m.ev)
			}
			return m, nil
		case "ctrl+t":
			pool := m.writable
			if len(pool) == 0 {
				pool = m.cals
			}
			if len(pool) > 0 {
				i := -1
				for j, c := range pool {
					if c == m.curCal {
						i = j
					}
				}
				m.curCal = pool[(i+1)%len(pool)]
				m.status = magenta.Render("calendar → " + m.curCal)
			}
			return m, m.reparse()
		case "ctrl+l":
			m.status = dim.Render("refreshed")
			return m, m.reload()
		case "ctrl+z":
			if m.vi.Undo(&m.input) {
				m.vi.clamp(&m.input)
				return m, m.reparse()
			}
			return m, nil
		case "ctrl+a":
			if !m.aiOn {
				m.status = yellow.Render("AI off (aichat missing or QUICKCAL_AI=off)")
				return m, nil
			}
			if m.ev != nil {
				delete(m.aiCache, aiInput(m.input.Value())) // force a fresh answer
			}
			return m, m.maybeAI(true)
		case "ctrl+o":
			return m, toggleIkhal(m.ikhalID)
		case "esc":
			if m.vi.mode == insertMode {
				m.vi.EnterNormal(&m.input)
				return m, m.syncPrompt()
			}
			m.status = ""
		}
		// esc+key arriving together is read as alt+key: treat it as esc, then key
		if m.vi.mode == insertMode && msg.Alt && msg.Type == tea.KeyRunes {
			m.vi.EnterNormal(&m.input)
			msg.Alt = false
		}
		if m.vi.mode == normalMode {
			// keys typed fast arrive as one multi-rune event: replay one by one,
			// switching to the text input if a key enters insert mode
			if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
				changed := false
				for i, r := range msg.Runes {
					if m.vi.mode == insertMode {
						rest := tea.KeyMsg{Type: tea.KeyRunes, Runes: msg.Runes[i:]}
						m.input, _ = m.input.Update(rest)
						changed = true
						break
					}
					changed = m.vi.Normal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, &m.input) || changed
				}
				if changed {
					return m, tea.Batch(m.syncPrompt(), m.reparse())
				}
				return m, m.syncPrompt()
			}
			changed := m.vi.Normal(msg, &m.input)
			if changed {
				return m, tea.Batch(m.syncPrompt(), m.reparse())
			}
			return m, m.syncPrompt()
		}
	}

	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() == before { // cursor blink, arrows…: nothing to reparse
		return m, cmd
	}
	return m, tea.Batch(cmd, m.reparse())
}

// syncPrompt shows the vi mode in the prompt and cursor: steady block in
// normal mode (moves are done via SetCursor, which doesn't reset the blink
// timer, so a blinking cursor would vanish after motions), blinking in insert.
func (m *model) syncPrompt() tea.Cmd {
	want := cursor.CursorBlink
	if m.vi.mode == normalMode {
		m.input.Prompt, m.input.PromptStyle = "● ", yellow
		want = cursor.CursorStatic
	} else {
		m.input.Prompt, m.input.PromptStyle = "❯ ", green
	}
	if m.input.Cursor.Mode() == want {
		return nil
	}
	return m.input.Cursor.SetMode(want)
}

// ------------------------------------------------------------------ view

func (m model) View() string {
	if m.w == 0 {
		return "loading…"
	}
	W := m.cw()
	entry := box("12").Width(W - 2).Render(m.input.View())
	hint := fit(dim.Render(" try: 'Dentist next fri 3pm remind 30m' · 'Gym every mon 7pm /work' · ':: notes'"), W)

	border := "4"
	if len(m.conflictSet()) > 0 {
		border = "1"
	}
	preview := box(border).Width(W - 2).Render(m.previewText(W))
	cmdLine := ""
	if m.ev != nil {
		cmdLine = dim.Width(W).Render("$ " + shellJoin(m.ev.KhalArgs()))
	}
	month := box("8").Render(monthGrid(m.focus, today()))
	dayTxt := dim.Render("…")
	if evs, ok := m.dayCache[m.focus.Format("2006-01-02")]; ok {
		dayTxt = renderDay(evs, m.conflictSet())
	}
	monthW := lipgloss.Width(month)
	innerH := lipgloss.Height(month) - 2
	// grow the day pane into spare rows (full-screen windows)
	fixed := 3 + 1 + lipgloss.Height(preview) + lipgloss.Height(cmdLine) + lipgloss.Height(month) + 2
	innerH += min(max(m.h-fixed-2, 0), 16)
	dayLines := strings.Split(fit(dayTxt, W-monthW-4), "\n")
	if len(dayLines) > innerH-1 {
		dayLines = append(dayLines[:innerH-2], dim.Render(fmt.Sprintf("+%d more", len(dayLines)-innerH+2)))
	}
	dayBox := box("8").Width(W - monthW - 2).Height(innerH).
		Render(bold.Render(m.focus.Format("Mon 02 Jan")) + "\n" + strings.Join(dayLines, "\n"))
	body := lipgloss.JoinVertical(lipgloss.Left, preview, cmdLine,
		lipgloss.JoinHorizontal(lipgloss.Top, month, dayBox))
	ik := "open"
	if m.ikhalID != "" {
		ik = "close"
	}
	keys := yellow.Render("-- INSERT --") + dim.Render(" enter add · esc normal · ^z undo · ^t cal · ^a AI · ^o "+ik+" ikhal · ^l refresh · ^q quit")
	if m.vi.mode == normalMode {
		pend := ""
		if len(m.vi.pending) > 0 || m.vi.count > 0 {
			pend = " " + bold.Render(strings.TrimLeft(fmt.Sprint(m.vi.count), "0")+string(m.vi.pending))
		}
		keys = yellow.Render("-- NORMAL --") + pend + dim.Render(" enter add · i insert · u/^r undo/redo · dd clear · ^t cal · ^a AI · ^o "+ik+" ikhal · ^l refresh")
	}
	help := fit(" "+keys, W)
	ui := lipgloss.JoinVertical(lipgloss.Left, entry, hint, body, " "+fit(m.status, W-1), help)
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, ui)
}

func (m model) previewText(W int) string {
	ev := m.ev
	if ev == nil {
		return dim.Render("Type an event above…\n\n\n\n\n")
	}
	var b strings.Builder
	if ev.Title != "" {
		b.WriteString(bold.Render(ev.Title))
	} else {
		b.WriteString(dim.Italic(true).Render("Untitled"))
	}
	b.WriteString("\n\n")
	if s, ok := ev.Start(); ok {
		e := ev.End()
		endS := e.Format("15:04")
		if e.YearDay() != s.YearDay() {
			endS = e.Format("Mon 02 Jan 15:04")
		}
		d := e.Sub(s)
		dur := ""
		if h := int(d.Hours()); h > 0 {
			dur += fmt.Sprintf("%dh", h)
		}
		if mi := int(d.Minutes()) % 60; mi > 0 {
			dur += fmt.Sprintf("%dm", mi)
		}
		b.WriteString("📅 " + cyan.Render(s.Format("Mon, 02 Jan 2006")) + "\n")
		b.WriteString("🕒 " + cyan.Render(s.Format("15:04")+" → "+endS) + dim.Render(" ("+dur+")") + "\n")
	} else {
		when := ev.StartDate.Format("Mon, 02 Jan 2006")
		if !ev.EndDate.IsZero() && !ev.EndDate.Equal(ev.StartDate) {
			when += " → " + ev.EndDate.Format("Mon, 02 Jan 2006")
		}
		b.WriteString("📅 " + cyan.Render(when) + dim.Render("  all-day") + "\n")
	}
	if ev.Location != "" {
		b.WriteString("📍 " + green.Render(ev.Location) + "\n")
	}
	b.WriteString("📒 " + magenta.Render(ev.Calendar) + "\n")
	if ev.Repeat != "" {
		r := ev.Repeat
		if !ev.Until.IsZero() {
			r += " until " + ev.Until.Format("2006-01-02")
		}
		b.WriteString("🔁 " + yellow.Render(r) + "\n")
	}
	if ev.Alarm != "" {
		b.WriteString("⏰ " + yellow.Render(ev.Alarm+" before") + "\n")
	}
	if ev.Description != "" {
		b.WriteString("📝 " + dim.Render(ev.Description) + "\n")
	}
	switch m.aiState {
	case "":
	case "done":
		b.WriteString(magenta.Render("✨ parsed by aichat") + "\n")
	case "waiting", "asking":
		b.WriteString(dim.Render("✨ asking aichat…") + "\n")
	default:
		b.WriteString(fit(yellow.Render("✨ "+m.aiState), W-4) + "\n")
	}
	for _, n := range ev.Notes {
		b.WriteString(yellow.Bold(true).Render("⚠ "+n) + "\n")
	}
	if _, timed := ev.Start(); timed {
		conf, missing := findConflicts(ev, m.dayCache)
		switch {
		case len(conf) > 0:
			b.WriteString("\n" + red.Bold(true).Render(fmt.Sprintf("⚠ conflicts with %d event(s):", len(conf))) + "\n")
			for _, k := range conf {
				line := "  " + red.Render(fmtSpan(k, ev.StartDate)) + " " + k.Color + "[" + k.Cal + "]\x1b[0m " + k.Title
				b.WriteString(fit(line, W-4) + "\n")
			}
		case len(missing) > 0:
			b.WriteString(dim.Render("checking conflicts…") + "\n")
		default:
			b.WriteString(green.Render("✓ no conflicts") + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// conflictSet keys existing events that clash with the current input.
func (m model) conflictSet() map[string]bool {
	set := map[string]bool{}
	if m.ev == nil {
		return set
	}
	conf, _ := findConflicts(m.ev, m.dayCache)
	for _, k := range conf {
		set[k.Cal+"|"+k.Title+"|"+k.Start.String()] = true
	}
	return set
}

// fmtSpan shows HH:MM-HH:MM, adding the date when the event spans other days.
func fmtSpan(k kEvent, day time.Time) string {
	if k.AllDay {
		return "all-day"
	}
	f := func(t time.Time) string {
		if t.Year() == day.Year() && t.YearDay() == day.YearDay() {
			return t.Format("15:04")
		}
		return t.Format("01/02 15:04")
	}
	return f(k.Start) + "-" + f(k.End)
}

func renderDay(evs []kEvent, clash map[string]bool) string {
	if len(evs) == 0 {
		return green.Render("free ✓")
	}
	var lines []string
	for _, k := range evs {
		span := k.Color + fmtSpan(k, k.Start) + "\x1b[0m"
		if clash[k.Cal+"|"+k.Title+"|"+k.Start.String()] {
			span = red.Bold(true).Render("✗ " + fmtSpan(k, k.Start))
		}
		lines = append(lines, span+" "+k.Title)
	}
	return strings.Join(lines, "\n")
}

func monthGrid(focus, today time.Time) string {
	var b strings.Builder
	b.WriteString(bold.Render(fmt.Sprintf("%-20s", centre(focus.Format("January 2006"), 20))) + "\n")
	b.WriteString(dim.Render("Mo Tu We Th Fr Sa Su") + "\n")
	first := time.Date(focus.Year(), focus.Month(), 1, 0, 0, 0, 0, focus.Location())
	d := first.AddDate(0, 0, -((int(first.Weekday()) + 6) % 7)) // back to Monday
	for d.Month() == focus.Month() || d.Before(first) {
		for i := range 7 {
			st := lipgloss.NewStyle()
			switch {
			case d.Equal(focus):
				st = st.Bold(true).Reverse(true).Foreground(lipgloss.Color("6"))
			case d.Equal(today):
				st = st.Bold(true).Underline(true)
			case d.Month() != focus.Month():
				st = dim
			case i >= 5:
				st = red
			}
			b.WriteString(st.Render(fmt.Sprintf("%2d", d.Day())))
			if i < 6 {
				b.WriteString(" ")
			}
			d = d.AddDate(0, 0, 1)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func centre(s string, w int) string {
	pad := max(w-len(s), 0)
	return strings.Repeat(" ", pad/2) + s
}

func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " '\"$`\\!*?&;|<>()#~") {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}

// ------------------------------------------------------------------ cli

func main() {
	fixPath()
	if len(os.Args) > 1 && os.Args[1] == "--fit" {
		fitMode = true
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-n", "--dry-run", "-x", "--exec":
			cals, def := loadCalendars()
			ev := parse.Parse(strings.Join(os.Args[2:], " "), cals, def, time.Now())
			args := ev.KhalArgs()
			fmt.Println(shellJoin(args))
			for _, n := range ev.Notes {
				fmt.Fprintln(os.Stderr, "warning:", n)
			}
			if os.Args[1] == "-x" || os.Args[1] == "--exec" {
				c := exec.Command(args[0], args[1:]...)
				c.Stdout, c.Stderr = os.Stdout, os.Stderr
				if err := c.Run(); err != nil {
					os.Exit(1)
				}
			}
			return
		case "-h", "--help":
			fmt.Println("usage: quickcal            TUI\n       quickcal --fit      TUI, resizing its kitty window to fit\n       quickcal -n TEXT    dry run (print khal command)\n       quickcal -x TEXT    create event")
			return
		}
	}
	if _, err := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithReportFocus()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
