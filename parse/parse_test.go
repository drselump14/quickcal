package parse

import (
	"strings"
	"testing"
	"time"
)

var (
	cals = []string{"personal", "personal-school", "japan-holidays", "family", "work", "icloud-home"}
	now  = time.Date(2026, 11, 23, 8, 42, 0, 0, time.Local) // Monday
)

func TestParse(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Lunch with Ana at Sushiro tomorrow 12:30 for 90m /work", "-a work -l Sushiro 2026-11-24 12:30 2026-11-24 14:00 Lunch with Ana"},
		{"Dentist next fri 3pm remind 30m", "-a personal -m 30m 2026-11-27 15:00 2026-11-27 16:00 Dentist"},
		{"Vacation dec 24 - dec 28 /icloud", "-a icloud-home 2026-12-24 2026-12-28 Vacation"},
		{"Gym every monday 7pm until 2027-03-01", "-a personal -r weekly -u 2027-03-01 2026-11-23 19:00 2026-11-23 20:00 Gym"},
		{"Call mom in 2 hours", "-a personal 2026-11-23 10:42 2026-11-23 11:42 Call mom"},
		{"Review PR 15:00-16:30 :: check migrations", "-a personal 2026-11-23 15:00 2026-11-23 16:30 Review PR :: check migrations"},
		{"Meeting 3-4pm", "-a personal 2026-11-23 15:00 2026-11-23 16:00 Meeting"},
		{"Sync from 11 to 1 wed", "-a personal 2026-11-25 11:00 2026-11-25 13:00 Sync"},
		{"Coffee at 3 at Blue Bottle", "-a personal -l Blue Bottle 2026-11-23 15:00 2026-11-23 16:00 Coffee"},
		{"Dinner tonight", "-a personal 2026-11-23 20:00 2026-11-23 21:00 Dinner"},
		{"Standup daily 9:30-9:45 /work", "-a work -r daily 2026-11-23 09:30 2026-11-23 09:45 Standup"},
		{"Concert on 12/5 at 7:30pm at Budokan", "-a personal -l Budokan 2026-12-05 19:30 2026-12-05 20:30 Concert"},
		{"Trip 3rd jan 2027 for 3 days", "-a personal 2027-01-03 2027-01-05 Trip"},
		{"Pay rent on the 1st monthly", "-a personal -r monthly 2026-12-01 Pay rent"},
		{"Breakfast 7am", "-a personal 2026-11-24 07:00 2026-11-24 08:00 Breakfast"},
		{"Report due friday", "-a personal 2026-11-27 Report due"},
		{"Team offsite next week", "-a personal 2026-11-30 Team offsite"},
		{"Read in the evening", "-a personal 2026-11-23 19:00 2026-11-23 20:00 Read"},
		{"Late call 23:00-01:00", "-a personal 2026-11-23 23:00 2026-11-24 01:00 Late call"},
		{"attend workshop thu", "-a personal 2026-11-26 attend workshop"},
		{"Dinner at 3 people", "-a personal 2026-11-23 Dinner at 3 people"},
		{"Planning 11-1pm", "-a personal 2026-11-23 11:00 2026-11-23 13:00 Planning"},
	}
	for _, c := range cases {
		got := strings.Join(Parse(c.in, cals, "personal", now).KhalArgs()[2:], " ")
		if got != c.want {
			t.Errorf("%q\n  got  %s\n  want %s", c.in, got, c.want)
		}
	}
}

func TestUnknownCalendarNote(t *testing.T) {
	ev := Parse("thing /nope", cals, "personal", now)
	if ev.Calendar != "personal" || len(ev.Notes) == 0 {
		t.Fatalf("got cal=%s notes=%v", ev.Calendar, ev.Notes)
	}
}
