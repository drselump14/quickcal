package main

// AI fallback via aichat: used only when the regex parser likely missed
// something (non-English text, Indonesian date words, leftover numbers).
//
//	QUICKCAL_AI=off          disable
//	QUICKCAL_AI_MODEL=...    aichat model (default: aichat's own default)

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/drselump14/quickcal/parse"
)

const aiDebounce = 700 * time.Millisecond

type aiResult struct {
	Title    string  `json:"title"`
	Date     string  `json:"date"`
	EndDate  *string `json:"end_date"`
	Start    *string `json:"start"`
	End      *string `json:"end"`
	Location *string `json:"location"`
}

var (
	reCalTok = regexp.MustCompile(`(^|\s)[/#][\w.-]+`)
	reDigit  = regexp.MustCompile(`\d`)
	// Indonesian date/time words the regex parser doesn't know
	reIDWords = regexp.MustCompile(`(?i)\b(besok|lusa|nanti|pagi|siang|sore|malam|jam|pukul|minggu|depan|senin|selasa|rabu|kamis|jumat|sabtu|hari|bulan|tanggal|tgl)\b`)
)

func aiEnabled() bool {
	if strings.EqualFold(os.Getenv("QUICKCAL_AI"), "off") {
		return false
	}
	_, err := exec.LookPath("aichat")
	return err == nil
}

// aiInput is what gets sent: no calendar token, no `:: description`.
func aiInput(raw string) string {
	if i := strings.Index(raw, "::"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(reCalTok.ReplaceAllString(raw, " "))
}

// needsAI decides whether the regex result is likely incomplete.
func needsAI(raw string, ev *parse.Event) bool {
	text := aiInput(raw)
	if text == "" || ev == nil {
		return false
	}
	for _, r := range text {
		if r > unicode.MaxASCII && unicode.IsLetter(r) { // CJK, accented, …
			return true
		}
	}
	return reDigit.MatchString(ev.Title) || reIDWords.MatchString(ev.Title)
}

func aiPrompt(text string, now time.Time) string {
	return fmt.Sprintf(`Now: %s (%s), timezone %s. Weeks start Monday.
Extract ONE calendar event from the text below. Reply with ONLY minified JSON, no prose, no code fence:
{"title":string,"date":"YYYY-MM-DD","end_date":"YYYY-MM-DD"|null,"start":"HH:MM"|null,"end":"HH:MM"|null,"location":string|null}
Rules: 24h times; start null = all-day; end_date only for multi-day events; dates in the past mean the next occurrence;
title = event name only, original language, without date/time/location words.
Text: %s`, now.Format("2006-01-02 15:04"), now.Format("Monday"), now.Location(), text)
}

// extractJSON takes the outermost {...}, tolerating fences or chatter.
func extractJSON(out string) (*aiResult, error) {
	i, j := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if i < 0 || j < i {
		return nil, fmt.Errorf("no JSON in reply: %.60q", out)
	}
	var r aiResult
	if err := json.Unmarshal([]byte(out[i:j+1]), &r); err != nil {
		return nil, fmt.Errorf("bad JSON: %v", err)
	}
	return &r, nil
}

func aiParse(text string, now time.Time) (*aiResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	args := []string{"-S"}
	if m := os.Getenv("QUICKCAL_AI_MODEL"); m != "" {
		args = append(args, "-m", m)
	}
	args = append(args, aiPrompt(text, now))
	out, err := exec.CommandContext(ctx, "aichat", args...).CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return nil, fmt.Errorf("aichat: %s", lines[0])
	}
	return extractJSON(string(out))
}

func hhmm(s *string) (int, bool) {
	if s == nil || *s == "" {
		return -1, false
	}
	t, err := time.Parse("15:04", *s)
	if err != nil {
		return -1, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// applyAI overlays AI date/time/title/location on the regex result, keeping
// calendar, repeat, alarm, duration and description from the regex parser.
func applyAI(base *parse.Event, r *aiResult) (*parse.Event, error) {
	loc := base.StartDate.Location()
	d, err := time.ParseInLocation("2006-01-02", r.Date, loc)
	if err != nil {
		return nil, fmt.Errorf("AI gave bad date %q", r.Date)
	}
	ev := *base
	ev.Notes = nil
	ev.StartDate, ev.EndDate = d, time.Time{}
	if r.EndDate != nil {
		if ed, err := time.ParseInLocation("2006-01-02", *r.EndDate, loc); err == nil && !ed.Equal(d) {
			ev.EndDate = ed
		}
	}
	ev.StartMin, _ = hhmm(r.Start)
	ev.EndMin, _ = hhmm(r.End)
	ev.AllDay = ev.StartMin < 0
	if ev.AllDay {
		ev.EndMin = -1
	}
	if t := strings.TrimSpace(r.Title); t != "" {
		ev.Title = t
	}
	if r.Location != nil {
		ev.Location = strings.TrimSpace(*r.Location)
	}
	return &ev, nil
}
