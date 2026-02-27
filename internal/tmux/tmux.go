package tmux

import (
	"os/exec"
	"strconv"
	"strings"
)

// HasSession returns true if a tmux session with the given name exists.
func HasSession(name string) bool {
	// tmux has-session exits 0 if the session exists, non-zero otherwise.
	err := exec.Command("tmux", "has-session", "-t", name).Run()
	return err == nil
}

// NewSession creates a new detached tmux session.
func NewSession(name, startDir string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", name, "-c", startDir).Run()
}

// NewWindow creates a new window in an existing session.
// target is "session:index" (e.g. "front:2").
func NewWindow(target, startDir string) error {
	return exec.Command("tmux", "new-window", "-t", target, "-c", startDir).Run()
}

// SendKeys sends keystrokes to a tmux target (e.g. "front:1").
func SendKeys(target, keys string) error {
	return exec.Command("tmux", "send-keys", "-t", target, keys, "C-m").Run()
}

// SelectWindow selects a window in a session (e.g. "front:1").
func SelectWindow(target string) error {
	return exec.Command("tmux", "select-window", "-t", target).Run()
}

// ListSessions returns the names of all running tmux sessions.
func ListSessions() ([]string, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// SwitchClient switches the current tmux client to the named session.
func SwitchClient(name string) error {
	return exec.Command("tmux", "switch-client", "-t", name).Run()
}

// DisplayPopup opens a tmux popup anchored to the top-left corner.
func DisplayPopup(title string, width, height int, style, command string) error {
	return exec.Command("tmux", "display-popup",
		"-T", title,
		"-x", "0",
		"-y", strconv.Itoa(height+1),
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
		"-S", style,
		"-E", command,
	).Run()
}
