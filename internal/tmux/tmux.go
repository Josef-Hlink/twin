package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
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

// ListSessions returns the names of all running tmux sessions, sorted by
// creation time (oldest first).
func ListSessions() ([]string, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_created} #{session_name}").Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	lines := strings.Split(raw, "\n")
	sort.Strings(lines) // timestamp prefix ensures chronological order

	names := make([]string, len(lines))
	for i, line := range lines {
		_, name, _ := strings.Cut(line, " ")
		names[i] = name
	}
	return names, nil
}

// CurrentSession returns the name of the currently attached tmux session.
func CurrentSession() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SwitchClient switches the current tmux client to the named session.
func SwitchClient(name string) error {
	return exec.Command("tmux", "switch-client", "-t", name).Run()
}

// ListWindows returns the windows in a session, formatted as "index:name"
// with tmux flags (e.g. * for active, - for last).
func ListWindows(session string) ([]string, error) {
	out, err := exec.Command("tmux", "list-windows", "-t", session,
		"-F", "#{window_index}:#{window_name}#{window_flags}").Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
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

// InTmux returns true if the current process is running inside tmux.
func InTmux() bool {
	return os.Getenv("TMUX") != ""
}

// AttachSession attaches the terminal to the named tmux session.
// Stdin/stdout/stderr are connected so tmux takes over the terminal.
func AttachSession(name string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Pane represents a single tmux pane.
type Pane struct {
	SessionName string
	WindowIndex int
	WindowName  string
	PaneIndex   int
	PanePID     int
	Command     string
	Target      string // "session:window.pane"
}

// ListAllPanes returns every pane across all tmux sessions.
func ListAllPanes() ([]Pane, error) {
	out, err := exec.Command("tmux", "list-panes", "-a",
		"-F", "#{session_name}\t#{window_index}\t#{window_name}\t#{pane_index}\t#{pane_pid}\t#{pane_current_command}").Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	var panes []Pane
	for line := range strings.SplitSeq(raw, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			continue
		}
		wIdx, _ := strconv.Atoi(fields[1])
		pIdx, _ := strconv.Atoi(fields[3])
		pid, _ := strconv.Atoi(fields[4])
		panes = append(panes, Pane{
			SessionName: fields[0],
			WindowIndex: wIdx,
			WindowName:  fields[2],
			PaneIndex:   pIdx,
			PanePID:     pid,
			Command:     fields[5],
			Target:      fmt.Sprintf("%s:%d.%d", fields[0], wIdx, pIdx),
		})
	}
	return panes, nil
}

// CapturePane captures the visible content of a tmux pane, preserving ANSI escape sequences.
func CapturePane(target string) (string, error) {
	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-p", "-e").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ClientSize returns the width and height of the current tmux client.
func ClientSize() (width, height int, err error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_width}\t#{client_height}").Output()
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("unexpected output: %s", out)
	}
	w, _ := strconv.Atoi(fields[0])
	h, _ := strconv.Atoi(fields[1])
	return w, h, nil
}

// DisplayPopupCenter opens a tmux popup centered on the screen.
func DisplayPopupCenter(title string, width, height int, style, command string) error {
	clientW, clientH, _ := ClientSize()
	x := max((clientW-width)/2, 0)
	y := max((clientH-height)/2, 0)

	return exec.Command("tmux", "display-popup",
		"-T", title,
		"-x", strconv.Itoa(x),
		"-y", strconv.Itoa(y),
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
		"-S", style,
		"-E", command,
	).Run()
}

// DisplayPopupRight opens a tmux popup anchored to the right side, vertically centered.
func DisplayPopupRight(title string, width, height int, style, command string) error {
	clientW, clientH, _ := ClientSize()
	x := max(clientW-width, 0)
	y := max((clientH-height)/2, 0)

	return exec.Command("tmux", "display-popup",
		"-T", title,
		"-x", strconv.Itoa(x),
		"-y", strconv.Itoa(y),
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
		"-S", style,
		"-E", command,
	).Run()
}
