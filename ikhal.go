package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ikhal pane: kitty split via remote control, or a suspended full-screen run
// outside kitty. QUICKCAL_IKHAL_AT picks where: vsplit (default), hsplit,
// tab, os-window, overlay.

type ikhalMsg struct {
	id     string // kitty window id of the launched pane
	closed bool
	err    error
}

type ikhalDoneMsg struct{ err error }

// --fit: size our own kitty OS window to the UI (and widen it for ikhal).
var fitMode bool

const (
	fitCols   = 84
	fitRows   = 34
	ikhalCols = 90
)

func resizeOS(cols, rows int) {
	if !fitMode || !inKitty() {
		return
	}
	_ = exec.Command("kitten", "@", "resize-os-window", "--self", "--unit=cells",
		"--width", fmt.Sprint(cols), "--height", fmt.Sprint(rows)).Run()
	centerOS()
}

// centerOS moves our kitty OS window to the middle of its display via yabai
// (kitty can resize but not move windows). kitty's platform_window_id is the
// macOS window number yabai uses. Silently does nothing without yabai.
func centerOS() {
	if _, err := exec.LookPath("yabai"); err != nil {
		return
	}
	out, err := exec.Command("kitten", "@", "ls", "--self").Output()
	if err != nil {
		return
	}
	var osWins []struct {
		PlatformWindowID int `json:"platform_window_id"`
	}
	if json.Unmarshal(out, &osWins) != nil || len(osWins) == 0 || osWins[0].PlatformWindowID == 0 {
		return
	}
	id := fmt.Sprint(osWins[0].PlatformWindowID)

	type frame struct{ X, Y, W, H float64 }
	var win struct {
		Frame   frame `json:"frame"`
		Display int   `json:"display"`
	}
	var disp struct {
		Frame frame `json:"frame"`
	}
	time.Sleep(150 * time.Millisecond) // let the resize land before reading the frame
	if out, err = exec.Command("yabai", "-m", "query", "--windows", "--window", id).Output(); err != nil ||
		json.Unmarshal(out, &win) != nil {
		return
	}
	if out, err = exec.Command("yabai", "-m", "query", "--displays", "--display", fmt.Sprint(win.Display)).Output(); err != nil ||
		json.Unmarshal(out, &disp) != nil {
		return
	}
	x := disp.Frame.X + (disp.Frame.W-win.Frame.W)/2
	y := disp.Frame.Y + (disp.Frame.H-win.Frame.H)/2
	_ = exec.Command("yabai", "-m", "window", id, "--move", fmt.Sprintf("abs:%d:%d", int(x), int(y))).Run()
}

func fitWindow(withIkhal bool) tea.Cmd {
	if !fitMode {
		return nil
	}
	return func() tea.Msg {
		cols := fitCols
		if withIkhal {
			cols += ikhalCols
		}
		resizeOS(cols, fitRows)
		return nil
	}
}

func inKitty() bool { return os.Getenv("KITTY_WINDOW_ID") != "" }

func launchArgs() []string {
	args := []string{"@", "launch", "--cwd=current", "--title=ikhal"}
	switch at := os.Getenv("QUICKCAL_IKHAL_AT"); at {
	case "tab", "os-window", "overlay":
		args = append(args, "--type="+at)
	case "hsplit":
		args = append(args, "--location=hsplit")
	default:
		args = append(args, "--location=vsplit")
	}
	return append(args, "ikhal")
}

// ikhalAlive reports closed=true when our pane was closed by other means.
func ikhalAlive(id string) tea.Cmd {
	if id == "" || !inKitty() {
		return nil
	}
	return func() tea.Msg {
		if exec.Command("kitten", "@", "ls", "--match", "id:"+id).Run() != nil {
			return ikhalMsg{closed: true}
		}
		return nil
	}
}

// toggleIkhal closes the pane we opened (if still alive) or opens a new one.
func toggleIkhal(openID string) tea.Cmd {
	if !inKitty() {
		return tea.ExecProcess(exec.Command("ikhal"), func(err error) tea.Msg { return ikhalDoneMsg{err} })
	}
	return func() tea.Msg {
		if openID != "" {
			if exec.Command("kitten", "@", "close-window", "--match", "id:"+openID).Run() == nil {
				return ikhalMsg{closed: true}
			}
		}
		resizeOS(fitCols+ikhalCols, fitRows) // make room before splitting
		out, err := exec.Command("kitten", launchArgs()...).CombinedOutput()
		if err != nil {
			return ikhalMsg{err: &khalErr{msg: "kitty: " + strings.TrimSpace(string(out)), err: err}}
		}
		return ikhalMsg{id: strings.TrimSpace(string(out))}
	}
}
