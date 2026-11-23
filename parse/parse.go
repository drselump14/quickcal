// Package parse turns natural-language event text into khal `new` arguments.
//
//	Lunch with Ana at Sushiro tomorrow 12:30 for 90m /work
//	Dentist next fri 3pm remind 30m
//	Vacation dec 24 - dec 28 /icloud
//	Gym every monday 7pm until 2027-03-01
//	Call mom in 2 hours
//	Review PR 15:00-16:30 :: check migrations
package parse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Event is the parse result. StartMin/EndMin are minutes since midnight, -1 = unset.
type Event struct {
	Title       string
	StartDate   time.Time // midnight, local
	EndDate     time.Time // zero = none
	StartMin    int
	EndMin      int
	Duration    time.Duration
	AllDay      bool
	Location    string
	Calendar    string
	Repeat      string
	Until       time.Time
	Alarm       string
	Description string
	Notes       []string
}

const isoDate = "2006-01-02"

// Start returns the start datetime; ok=false for all-day events.
func (e *Event) Start() (time.Time, bool) {
	if e.AllDay || e.StartMin < 0 {
		return time.Time{}, false
	}
	return e.StartDate.Add(time.Duration(e.StartMin) * time.Minute), true
}

// End returns the end datetime of a timed event (default 1h).
func (e *Event) End() time.Time {
	s, _ := e.Start()
	if e.EndMin >= 0 {
		base := e.StartDate
		if !e.EndDate.IsZero() {
			base = e.EndDate
		}
		end := base.Add(time.Duration(e.EndMin) * time.Minute)
		if !end.After(s) { // 23:00-01:00 crosses midnight
			end = end.AddDate(0, 0, 1)
		}
		return end
	}
	if e.Duration > 0 {
		return s.Add(e.Duration)
	}
	return s.Add(time.Hour)
}

// KhalArgs builds the argv for `khal new`.
func (e *Event) KhalArgs() []string {
	args := []string{"khal", "new"}
	if e.Calendar != "" {
		args = append(args, "-a", e.Calendar)
	}
	if e.Location != "" {
		args = append(args, "-l", e.Location)
	}
	if e.Repeat != "" {
		args = append(args, "-r", e.Repeat)
		if !e.Until.IsZero() {
			args = append(args, "-u", e.Until.Format(isoDate))
		}
	}
	if e.Alarm != "" {
		args = append(args, "-m", e.Alarm)
	}
	if s, ok := e.Start(); ok {
		en := e.End()
		args = append(args, s.Format(isoDate), s.Format("15:04"), en.Format(isoDate), en.Format("15:04"))
	} else {
		args = append(args, e.StartDate.Format(isoDate))
		if !e.EndDate.IsZero() && !e.EndDate.Equal(e.StartDate) {
			args = append(args, e.EndDate.Format(isoDate))
		}
	}
	title := e.Title
	if title == "" {
		title = "Untitled"
	}
	args = append(args, title)
	if e.Description != "" {
		args = append(args, "::", e.Description)
	}
	return args
}

// ------------------------------------------------------------------ tables

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday,
}

var months = map[string]time.Month{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3,
	"apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7,
	"aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9,
	"oct": 10, "october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12,
}

var numWords = map[string]int{"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4,
	"five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10}

var fuzzyTimes = map[string]int{"morning": 9 * 60, "afternoon": 14 * 60, "evening": 19 * 60, "tonight": 20 * 60}

const (
	wdP  = `(?:mon(?:day)?|tues?(?:day)?|wed(?:s|nesday)?|thu(?:rs?)?(?:day)?|fri(?:day)?|sat(?:urday)?|sun(?:day)?)`
	monP = `(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|june?|july?|aug(?:ust)?|sep(?:t(?:ember)?)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)`
	ordP = `(?:st|nd|rd|th)?`
	numP = `(?:\d+|a|an|one|two|three|four|five|six|seven|eight|nine|ten)`
	// strong time: am/pm, colon, or named
	tStrong = `(?:\d{1,2}(?::\d{2})?\s*(?:am|pm|a|p)\b|\d{1,2}:\d{2}|noon|midnight)`
	tAny    = `(?:\d{1,2}(?::\d{2})?\s*(?:am|pm|a|p)?\b|noon|midnight)`
)

var dateP = `(?:` + strings.Join([]string{
	`\d{4}-\d{1,2}-\d{1,2}`,
	`\d{1,2}/\d{1,2}(?:/\d{2,4})?`,
	`\d{1,2}` + ordP + `\s+` + monP + `(?:,?\s+\d{4})?`,
	monP + `\.?\s+\d{1,2}` + ordP + `(?:,?\s+\d{4})?`,
	`day\s+after\s+tomorrow`,
	`today|tonight|tomorrow|tmrw?|tomorow|yesterday`,
	`(?:next|this)\s+` + wdP,
	wdP,
	`next\s+week|next\s+month|(?:this\s+)?weekend`,
	`in\s+` + numP + `\s+(?:days?|weeks?|months?)`,
	`(?:on\s+)?the\s+\d{1,2}(?:st|nd|rd|th)`,
}, "|") + `)`

func re(p string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + p) }

var (
	reCal       = re(`(?:^|\s)[/#]([\w.-]+)(?:\s|$)`)
	reAlarm     = re(`\b(?:remind(?:\s+me)?|alert|alarm)\s+(?:me\s+)?(\d+)\s*(m|mins?|minutes?|h|hrs?|hours?|d|days?)\b(?:\s+before)?`)
	reRepeat    = re(`\b(?:every\s+(day|week|month|year|` + wdP + `)|(daily|weekly|monthly|yearly|annually))\b`)
	reUntil     = re(`\b(?:until|till|through)\s+(` + dateP + `)\b`)
	reAllDay    = re(`\ball[\s-]?day\b`)
	reDurHM     = re(`\bfor\s+(\d+)\s*h(?:rs?|ours?)?\s*(\d+)\s*m(?:in(?:ute)?s?)?\b`)
	reDur       = re(`\bfor\s+(\d+(?:\.\d+)?|an?|one|two|three)\s*(h|hrs?|hours?|m|mins?|minutes?|d|days?)\b`)
	reRelTime   = re(`\bin\s+(\d+|an?|one|two|three)\s+(hours?|hrs?|minutes?|mins?)\b`)
	reRange     = re(`\b(?:from\s+)?(?:at\s+)?(` + tAny + `)\s*(?:-|–|to|until|till)\s*(` + tAny + `)`)
	reTStrong   = re(`^` + tStrong + `$`)
	reSingle    = re(`\b(?:at\s+|@\s*)?(` + tStrong + `)`)
	reBareAt    = re(`\bat\s+(\d{1,2})\b`)
	reBareNext  = re(`^(?:[/:.-]|\s+(?:days?|weeks?|months?|hours?|hrs?|mins?|minutes?|people|pax)\b)`)
	reDate      = re(`(?:\b(?:on|from|to|until|till|through|thru)\s+|[-–]\s*)?\b(` + dateP + `)\b`)
	reFuzzy     = re(`\b(?:in\s+the\s+|this\s+)?(morning|afternoon|evening|tonight)\b`)
	reLocation  = re(`(?:^|\s)(?:at\s+|@\s*)([^\d\s].*)$`)
	reTitleTrim = re(`^(?:on|at|from|to|-|,)\s+|\s+(?:on|at|from|to|for|-|,)$`)
	reSpaces    = regexp.MustCompile(`\s+`)
	reMerEnd    = re(`(am|pm|a|p)\s*$`)
	reMerAny    = re(`(am|pm|a|p|noon|midnight)\s*$`)
	reTime      = re(`^(\d{1,2})(?::(\d{2}))?\s*(am|pm|a|p)?$`)

	reISO    = re(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	reMDY    = re(`^(\d{1,2})/(\d{1,2})(?:/(\d{2,4}))?$`)
	reDMon   = re(`^(\d{1,2})` + ordP + ` (` + monP + `)(?:,? (\d{4}))?$`)
	reMonD   = re(`^(` + monP + `)\.? (\d{1,2})` + ordP + `(?:,? (\d{4}))?$`)
	reRelWd  = re(`^(next|this) (` + wdP + `)$`)
	reInN    = re(`^in (\w+) (day|week|month)s?$`)
	reTheNth = re(`^(?:on )?the (\d{1,2})(?:st|nd|rd|th)$`)
)

// ------------------------------------------------------------------ helpers

type text struct{ s string }

func (t *text) blank(a, b int) { t.s = t.s[:a] + strings.Repeat(" ", b-a) + t.s[b:] }

// take finds the first match, blanks it, returns submatches (nil if none).
func (t *text) take(r *regexp.Regexp) []string {
	idx := r.FindStringSubmatchIndex(t.s)
	if idx == nil {
		return nil
	}
	m := make([]string, len(idx)/2)
	for i := range m {
		if idx[2*i] >= 0 {
			m[i] = t.s[idx[2*i]:idx[2*i+1]]
		}
	}
	t.blank(idx[0], idx[1])
	return m
}

func num(tok string) int {
	if n, err := strconv.Atoi(tok); err == nil {
		return n
	}
	return numWords[strings.ToLower(tok)]
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func nextWeekday(today time.Time, wd time.Weekday, includeToday bool) time.Time {
	delta := (int(wd) - int(today.Weekday()) + 7) % 7
	if delta == 0 && !includeToday {
		delta = 7
	}
	return today.AddDate(0, 0, delta)
}

// addMonths clamps the day (Jan 31 + 1 = Feb 28), unlike time.AddDate.
func addMonths(d time.Time, n int) time.Time {
	first := time.Date(d.Year(), d.Month()+time.Month(n), 1, 0, 0, 0, 0, d.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(d.Day(), last)-1)
}

func mkDate(y int, m time.Month, d int, loc *time.Location) (time.Time, bool) {
	t := time.Date(y, m, d, 0, 0, 0, 0, loc)
	if t.Day() != d || t.Month() != m { // reject Feb 31 etc.
		return time.Time{}, false
	}
	return t, true
}

func future(d, today time.Time, explicitYear bool) time.Time {
	if !explicitYear && d.Before(today) {
		return d.AddDate(1, 0, 0)
	}
	return d
}

// ParseDate resolves a date phrase relative to today (midnight).
func ParseDate(phrase string, today time.Time) (time.Time, bool) {
	p := strings.TrimRight(reSpaces.ReplaceAllString(strings.ToLower(strings.TrimSpace(phrase)), " "), ".")
	loc := today.Location()
	if m := reISO.FindStringSubmatch(p); m != nil {
		return mkDate(num(m[1]), time.Month(num(m[2])), num(m[3]), loc)
	}
	if m := reMDY.FindStringSubmatch(p); m != nil { // M/D[/Y]
		yr := today.Year()
		if m[3] != "" {
			yr = num(m[3])
			if yr < 100 {
				yr += 2000
			}
		}
		d, ok := mkDate(yr, time.Month(num(m[1])), num(m[2]), loc)
		return future(d, today, m[3] != ""), ok
	}
	if m := reDMon.FindStringSubmatch(p); m != nil {
		yr := today.Year()
		if m[3] != "" {
			yr = num(m[3])
		}
		d, ok := mkDate(yr, months[m[2]], num(m[1]), loc)
		return future(d, today, m[3] != ""), ok
	}
	if m := reMonD.FindStringSubmatch(p); m != nil {
		yr := today.Year()
		if m[3] != "" {
			yr = num(m[3])
		}
		d, ok := mkDate(yr, months[m[1]], num(m[2]), loc)
		return future(d, today, m[3] != ""), ok
	}
	switch p {
	case "day after tomorrow":
		return today.AddDate(0, 0, 2), true
	case "today", "tonight":
		return today, true
	case "tomorrow", "tmr", "tmrw", "tomorow":
		return today.AddDate(0, 0, 1), true
	case "yesterday":
		return today.AddDate(0, 0, -1), true
	case "next week":
		return nextWeekday(today, time.Monday, false), true
	case "next month":
		return addMonths(today.AddDate(0, 0, 1-today.Day()), 1), true
	case "weekend", "this weekend":
		return nextWeekday(today, time.Saturday, true), true
	}
	if m := reRelWd.FindStringSubmatch(p); m != nil {
		return nextWeekday(today, weekdays[m[2]], m[1] == "this"), true
	}
	if wd, ok := weekdays[p]; ok {
		return nextWeekday(today, wd, true), true
	}
	if m := reInN.FindStringSubmatch(p); m != nil {
		n := num(m[1])
		switch m[2] {
		case "day":
			return today.AddDate(0, 0, n), true
		case "week":
			return today.AddDate(0, 0, 7*n), true
		default:
			return addMonths(today, n), true
		}
	}
	if m := reTheNth.FindStringSubmatch(p); m != nil {
		d, ok := mkDate(today.Year(), today.Month(), num(m[1]), loc)
		if ok && d.Before(today) {
			d = addMonths(d, 1)
		}
		return d, ok
	}
	return time.Time{}, false
}

// ParseTime returns minutes since midnight and whether am/pm was given.
// A bare hour 1-7 is assumed PM ("at 3" -> 15:00).
func ParseTime(tok string) (int, bool, error) {
	t := strings.ToLower(strings.TrimSpace(tok))
	switch t {
	case "noon":
		return 12 * 60, true, nil
	case "midnight":
		return 0, true, nil
	}
	m := reTime.FindStringSubmatch(t)
	if m == nil {
		return 0, false, fmt.Errorf("bad time %q", tok)
	}
	h, mi, mer := num(m[1]), 0, m[3]
	if m[2] != "" {
		mi = num(m[2])
	}
	if mer != "" {
		if h < 1 || h > 12 {
			return 0, false, fmt.Errorf("bad time %q", tok)
		}
		h %= 12
		if mer[0] == 'p' {
			h += 12
		}
	} else if m[2] == "" && h >= 1 && h <= 7 {
		h += 12
	}
	if h > 23 || mi > 59 {
		return 0, false, fmt.Errorf("bad time %q", tok)
	}
	return h*60 + mi, mer != "", nil
}

func resolveRange(a, b string) (int, int, error) {
	end, endMer, err := ParseTime(b)
	if err != nil {
		return 0, 0, err
	}
	if m := reMerEnd.FindStringSubmatch(strings.TrimSpace(b)); m != nil && !reMerAny.MatchString(strings.TrimSpace(a)) {
		// "3-4pm": start takes the same meridiem, else the opposite
		start, _, err := ParseTime(a + m[1])
		if err != nil {
			return 0, 0, err
		}
		if start > end {
			other := "pm"
			if strings.HasPrefix(strings.ToLower(m[1]), "p") {
				other = "am"
			}
			if start, _, err = ParseTime(a + other); err != nil {
				return 0, 0, err
			}
		}
		return start, end, nil
	}
	start, _, err := ParseTime(a)
	if err != nil {
		return 0, 0, err
	}
	if !endMer && !strings.Contains(b, ":") && end <= start && end+12*60 < 24*60 {
		end += 12 * 60
	}
	return start, end, nil
}

// ------------------------------------------------------------------ main

// Parse parses input text. now is injected for testability.
func Parse(input string, calendars []string, defaultCal string, now time.Time) *Event {
	today := midnight(now)
	ev := &Event{Calendar: defaultCal, StartMin: -1, EndMin: -1}

	if i := strings.Index(input, "::"); i >= 0 {
		ev.Description = strings.TrimSpace(input[i+2:])
		input = input[:i]
	}
	w := &text{" " + input + " "}

	// calendar: /name or #name -> exact, prefix, substring
	if m := w.take(reCal); m != nil {
		q := strings.ToLower(m[1])
		hit := ""
		for _, match := range []func(string) bool{
			func(c string) bool { return c == q },
			func(c string) bool { return strings.HasPrefix(c, q) },
			func(c string) bool { return strings.Contains(c, q) },
		} {
			for _, c := range calendars {
				if match(strings.ToLower(c)) {
					hit = c
					break
				}
			}
			if hit != "" {
				break
			}
		}
		if hit != "" {
			ev.Calendar = hit
		} else {
			ev.Notes = append(ev.Notes, fmt.Sprintf("unknown calendar '%s', using %s", q, defaultCal))
		}
	}

	if m := w.take(reAlarm); m != nil {
		ev.Alarm = m[1] + strings.ToLower(m[2][:1])
	}

	if m := w.take(reRepeat); m != nil {
		unit := strings.ToLower(m[1] + m[2])
		if wd, ok := weekdays[unit]; ok {
			ev.Repeat = "weekly"
			ev.StartDate = nextWeekday(today, wd, true)
		} else {
			ev.Repeat = map[string]string{"day": "daily", "week": "weekly", "month": "monthly",
				"year": "yearly", "annually": "yearly"}[unit]
			if ev.Repeat == "" {
				ev.Repeat = unit
			}
		}
		if mu := w.take(reUntil); mu != nil {
			if d, ok := ParseDate(mu[1], today); ok {
				ev.Until = d
			}
		}
	}

	if w.take(reAllDay) != nil {
		ev.AllDay = true
	}

	if m := w.take(reDurHM); m != nil {
		ev.Duration = time.Duration(num(m[1]))*time.Hour + time.Duration(num(m[2]))*time.Minute
	} else if m := w.take(reDur); m != nil {
		var n float64
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			n = f
		} else {
			n = float64(num(m[1]))
		}
		unit := map[byte]time.Duration{'h': time.Hour, 'm': time.Minute, 'd': 24 * time.Hour}[strings.ToLower(m[2])[0]]
		ev.Duration = time.Duration(n * float64(unit))
	}

	if m := w.take(reRelTime); m != nil {
		d := time.Duration(num(m[1])) * time.Minute
		if strings.HasPrefix(strings.ToLower(m[2]), "h") {
			d *= 60
		}
		dt := now.Add(d).Truncate(time.Minute)
		ev.StartDate, ev.StartMin = midnight(dt), dt.Hour()*60+dt.Minute()
	}

	// time range
	if ev.StartMin < 0 {
		for _, idx := range reRange.FindAllStringSubmatchIndex(w.s, -1) {
			whole := strings.ToLower(strings.TrimSpace(w.s[idx[0]:idx[1]]))
			a, b := w.s[idx[2]:idx[3]], w.s[idx[4]:idx[5]]
			if !(reTStrong.MatchString(strings.TrimSpace(a)) || reTStrong.MatchString(strings.TrimSpace(b)) ||
				strings.HasPrefix(whole, "from") || strings.HasPrefix(whole, "at")) {
				continue
			}
			s, e, err := resolveRange(a, b)
			if err != nil {
				continue
			}
			ev.StartMin, ev.EndMin = s, e
			w.blank(idx[0], idx[1])
			break
		}
	}

	// single time
	if ev.StartMin < 0 {
		for _, idx := range reSingle.FindAllStringSubmatchIndex(w.s, -1) {
			if t, _, err := ParseTime(w.s[idx[2]:idx[3]]); err == nil {
				ev.StartMin = t
				w.blank(idx[0], idx[1])
				break
			}
		}
	}
	if ev.StartMin < 0 { // "at 3", but not "at 3 people" / "at 3/4"
		for _, idx := range reBareAt.FindAllStringSubmatchIndex(w.s, -1) {
			if reBareNext.MatchString(w.s[idx[1]:]) {
				continue
			}
			if t, _, err := ParseTime(w.s[idx[2]:idx[3]]); err == nil {
				ev.StartMin = t
				w.blank(idx[0], idx[1])
				break
			}
		}
	}

	// dates: up to two -> range
	var dates []time.Time
	for range 2 {
		m := w.take(reDate)
		if m == nil {
			break
		}
		d, ok := ParseDate(m[1], today)
		if !ok {
			continue
		}
		dates = append(dates, d)
		if strings.EqualFold(strings.TrimSpace(m[1]), "tonight") && ev.StartMin < 0 {
			ev.StartMin = fuzzyTimes["tonight"]
		}
	}
	if len(dates) > 0 {
		ev.StartDate = dates[0]
		if len(dates) > 1 {
			ev.EndDate = dates[1]
			if ev.EndDate.Before(ev.StartDate) {
				ev.Notes = append(ev.Notes, "end date before start date")
			}
		}
	}

	if ev.StartMin < 0 {
		if m := w.take(reFuzzy); m != nil {
			k := strings.ToLower(m[1])
			ev.StartMin = fuzzyTimes[k]
			if k == "tonight" && ev.StartDate.IsZero() {
				ev.StartDate = today
			}
		}
	}

	// defaults
	if ev.StartDate.IsZero() {
		ev.StartDate = today
		if ev.StartMin >= 0 && ev.Repeat == "" &&
			today.Add(time.Duration(ev.StartMin)*time.Minute).Before(now) {
			ev.StartDate = today.AddDate(0, 0, 1)
			ev.Notes = append(ev.Notes, "time already passed today -> tomorrow")
		}
	}
	if ev.StartMin < 0 {
		ev.AllDay = true
	}
	if ev.AllDay {
		ev.StartMin, ev.EndMin = -1, -1
		if days := int(ev.Duration.Hours() / 24); days > 1 && ev.EndDate.IsZero() {
			ev.EndDate = ev.StartDate.AddDate(0, 0, days-1)
		}
	}

	// location: "at X" / "@X" on the remainder (X must not start with a digit)
	rest := strings.TrimSpace(reSpaces.ReplaceAllString(w.s, " "))
	if idx := reLocation.FindStringSubmatchIndex(rest); idx != nil {
		ev.Location = strings.Trim(rest[idx[2]:idx[3]], " ,.-")
		rest = strings.TrimSpace(rest[:idx[0]])
	}

	ev.Title = strings.Trim(reTitleTrim.ReplaceAllString(rest, ""), " ,-")
	if ev.Title == "" {
		ev.Notes = append(ev.Notes, "no title")
	}
	return ev
}
