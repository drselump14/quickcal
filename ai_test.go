package main

import (
	"testing"
	"time"

	"github.com/drselump14/quickcal/parse"
)

func TestNeedsAI(t *testing.T) {
	now := time.Date(2026, 11, 23, 8, 42, 0, 0, time.Local)
	cases := map[string]bool{
		"Lunch with Ana at Sushiro tomorrow 12:30 /work": false, // regex handles it
		"Dentist next fri 3pm remind 30m":                false,
		"Report due friday":                              false,
		"明日15時 歯医者":                                      true,  // Japanese
		"Rapat besok jam 3 sore":                         true,  // Indonesian words
		"Coffee 1530":                                    true,  // leftover number
		"Gym tomorrow 7pm :: 明日のメモ":                      false, // description is never sent
		"standup /personal-school":                       false,
	}
	for in, want := range cases {
		ev := parse.Parse(in, []string{"work", "personal-school"}, "personal", now)
		if got := needsAI(in, ev); got != want {
			t.Errorf("needsAI(%q) = %v, want %v (title %q)", in, got, want, ev.Title)
		}
	}
	if got := aiInput("会議 明日 /work :: secret notes"); got != "会議 明日" {
		t.Errorf("aiInput leaked: %q", got)
	}
}

func TestApplyAI(t *testing.T) {
	now := time.Date(2026, 11, 23, 8, 42, 0, 0, time.Local)
	base := parse.Parse("明日15時 歯医者 /work remind 30m :: card", []string{"work"}, "personal", now)
	r, err := extractJSON("```json\n" + `{"title":"歯医者","date":"2026-11-24","end_date":null,"start":"15:00","end":null,"location":null}` + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	ev, err := applyAI(base, r)
	if err != nil {
		t.Fatal(err)
	}
	got := ev.KhalArgs()[2:]
	want := []string{"-a", "work", "-m", "30m", "2026-11-24", "15:00", "2026-11-24", "16:00", "歯医者", "::", "card"}
	if len(got) != len(want) {
		t.Fatalf("got %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q\nwant %q", got, want)
		}
	}
	if _, err := applyAI(base, &aiResult{Title: "x", Date: "tomorrow"}); err == nil {
		t.Error("bad date should error")
	}
}
