package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func khal(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "khal", args...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// fixPath adds common install dirs when khal isn't found: GUI launchers
// (kitty panels, Dock) start with a minimal PATH.
func fixPath() {
	if _, err := exec.LookPath("khal"); err == nil {
		return
	}
	home, _ := os.UserHomeDir()
	extra := []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin")}
	os.Setenv("PATH", strings.Join(extra, ":")+":"+os.Getenv("PATH"))
}

func loadCalendars() (cals []string, def string) {
	out, _ := khal("printcalendars")
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			cals = append(cals, l)
		}
	}
	home, _ := os.UserHomeDir()
	cfg := filepath.Join(home, ".config", "khal", "config")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		cfg = filepath.Join(xdg, "khal", "config")
	}
	if b, err := os.ReadFile(cfg); err == nil {
		if m := regexp.MustCompile(`(?m)^\s*default_calendar\s*=\s*(\S+)`).FindSubmatch(b); m != nil {
			def = string(m[1])
		}
	}
	if def == "" && len(cals) > 0 {
		def = cals[0]
	}
	return cals, def
}

func createEvent(args []string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return &khalErr{msg: lines[len(lines)-1], err: err}
	}
	return nil
}

type khalErr struct {
	msg string
	err error
}

func (e *khalErr) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return e.err.Error()
}
