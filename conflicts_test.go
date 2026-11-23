package main

import (
	"testing"
	"time"

	"github.com/drselump14/quickcal/parse"
)

func TestFindConflicts(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 11, 23, 8, 0, 0, 0, loc)
	raw := "\x1b[33m\x1f2026-11-24\x1f11:00\x1f2026-11-24\x1f14:00\x1fFalse\x1fwork\x1fBig meeting\x1b[0m\n" +
		"\x1f2026-11-24\x1f\x1f2026-11-24\x1f\x1fTrue\x1fjapan-holidays\x1fHoliday\n" +
		"\x1f2026-11-24\x1f16:00\x1f2026-11-24\x1f16:45\x1fFalse\x1fpersonal\x1fClinic\n"
	cache := map[string][]kEvent{"2026-11-24": parseDayList(raw, loc)}
	if n := len(cache["2026-11-24"]); n != 3 {
		t.Fatalf("parsed %d events, want 3", n)
	}

	cases := []struct {
		in   string
		want []string // conflicting calendars
	}{
		{"Lunch tomorrow 12:30", []string{"work"}}, // inside 11-14
		{"Call tomorrow 14:00-16:00", nil},         // touches edges only
		{"Workshop tomorrow 13:00-17:00", []string{"work", "personal"}},
		{"Day off tomorrow", nil}, // all-day new event: no check
		{"Early tomorrow 9am", nil},
	}
	for _, c := range cases {
		ev := parse.Parse(c.in, nil, "personal", now)
		conf, missing := findConflicts(ev, cache)
		if len(missing) > 0 {
			t.Errorf("%q: unexpected missing days %v", c.in, missing)
		}
		var got []string
		for _, k := range conf {
			got = append(got, k.Cal)
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: got %v want %v", c.in, got, c.want)
			}
		}
	}

	// crossing midnight needs the next day too
	ev := parse.Parse("Late tomorrow 23:00-01:00", nil, "personal", now)
	if _, missing := findConflicts(ev, cache); len(missing) != 1 || missing[0].Day() != 25 {
		t.Errorf("midnight span: missing=%v, want [25th]", missing)
	}
}
