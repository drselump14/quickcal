package main

import (
	"strings"
	"time"

	"github.com/drselump14/quickcal/parse"
)

// kEvent is an existing khal event, as listed for one day.
type kEvent struct {
	Start, End time.Time // for all-day: dates at midnight
	AllDay     bool
	Cal        string
	Color      string // ANSI sequence from khal's {calendar-color}
	Title      string
}

const sep = "\x1f"

// fetchDay lists one day's events from khal in a parseable form.
func fetchDay(d time.Time) ([]kEvent, error) {
	format := strings.Join([]string{"{calendar-color}", "{start-date}", "{start-time}", "{end-date}",
		"{end-time}", "{all-day}", "{calendar}", "{title}"}, sep) + "{reset}"
	out, err := khal("--color", "list", "--format", format, "--day-format", "", d.Format("2006-01-02"), "1d")
	if err != nil {
		return nil, err
	}
	return parseDayList(out, d.Location()), nil
}

func parseDayList(out string, loc *time.Location) []kEvent {
	var evs []kEvent
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, sep)
		if len(f) != 8 {
			continue
		}
		e := kEvent{Color: f[0], AllDay: f[5] == "True", Cal: f[6],
			Title: strings.TrimSuffix(f[7], "\x1b[0m")}
		var err1, err2 error
		if e.AllDay {
			e.Start, err1 = time.ParseInLocation("2006-01-02", f[1], loc)
			e.End, err2 = time.ParseInLocation("2006-01-02", f[3], loc)
		} else {
			e.Start, err1 = time.ParseInLocation("2006-01-02 15:04", f[1]+" "+f[2], loc)
			e.End, err2 = time.ParseInLocation("2006-01-02 15:04", f[3]+" "+f[4], loc)
		}
		if err1 != nil || err2 != nil {
			continue
		}
		evs = append(evs, e)
	}
	return evs
}

// spanDays returns every date the new event touches (timed events only).
func spanDays(ev *parse.Event) []time.Time {
	if _, ok := ev.Start(); !ok {
		return nil
	}
	last := ev.End().Add(-time.Nanosecond)
	var days []time.Time
	for d := ev.StartDate; !d.After(last); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	return days
}

// findConflicts returns existing timed events overlapping ev, and the days
// not yet in cache. All-day events (holidays, OOO markers) never conflict.
func findConflicts(ev *parse.Event, cache map[string][]kEvent) (conf []kEvent, missing []time.Time) {
	s, ok := ev.Start()
	if !ok {
		return nil, nil
	}
	e := ev.End()
	seen := map[string]bool{}
	for _, d := range spanDays(ev) {
		evs, ok := cache[d.Format("2006-01-02")]
		if !ok {
			missing = append(missing, d)
			continue
		}
		for _, k := range evs {
			key := k.Cal + "|" + k.Title + "|" + k.Start.String()
			if k.AllDay || seen[key] {
				continue
			}
			if k.Start.Before(e) && s.Before(k.End) {
				seen[key] = true
				conf = append(conf, k)
			}
		}
	}
	return conf, missing
}
