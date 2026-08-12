package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// HasSession returns true if a tmux session with the given name exists.
func HasSession(name string) bool {
	// tmux has-session exits 0 if the session exists, non-zero otherwise.
	err := exec.Command("tmux", "has-session", "-t", name).Run()
	return err == nil
}

// NewSession creates a new detached tmux session and returns the pane-id
// (e.g. "%3") of its first window's base pane.
//
// The session is sized to the destination terminal via -x/-y. Without this a
// detached session falls back to tmux's 80x24 default-size, so pane split
// percentages are computed against that tiny grid; when the session is later
// attached to a larger terminal tmux spreads the extra space across panes,
// dragging every non-50% split toward 50%. Creating at the final size keeps
// the percentages honest. Falls back to tmux's default when the size is
// unknown (e.g. stdout isn't a terminal).
// windowName, when non-empty, names the first window; tmux then disables
// automatic-rename for it, so the name sticks.
func NewSession(name, startDir, windowName string) (string, error) {
	args := []string{"new-session", "-d", "-s", name, "-c", startDir}
	if windowName != "" {
		args = append(args, "-n", windowName)
	}
	if cols, rows := SessionSize(); cols > 0 && rows > 0 {
		args = append(args, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))
	}
	args = append(args, "-P", "-F", "#{pane_id}")
	return paneIDOf(exec.Command("tmux", args...))
}

// NewWindow creates a new window in an existing session and returns the pane-id
// of its base pane. target is "session:index" (e.g. "front:2"). windowName,
// when non-empty, names the window (disabling automatic-rename for it).
func NewWindow(target, startDir, windowName string) (string, error) {
	args := []string{"new-window", "-t", target, "-c", startDir}
	if windowName != "" {
		args = append(args, "-n", windowName)
	}
	args = append(args, "-P", "-F", "#{pane_id}")
	return paneIDOf(exec.Command("tmux", args...))
}

// SplitPane splits the pane identified by targetPaneID and returns the new
// pane's pane-id. direction is "down" for a stacked split (-v) or anything else
// for a side-by-side split (-h). size, when non-empty, is a tmux size such as
// "30%" (requires tmux >= 3.1); empty means an even split.
func SplitPane(targetPaneID, startDir, direction, size string) (string, error) {
	args := []string{"split-window", "-t", targetPaneID, "-c", startDir}
	if direction == "down" {
		args = append(args, "-v")
	} else {
		args = append(args, "-h")
	}
	if size != "" {
		args = append(args, "-l", size)
	}
	args = append(args, "-P", "-F", "#{pane_id}")
	return paneIDOf(exec.Command("tmux", args...))
}

// SelectPane focuses the pane identified by paneID (e.g. "%3").
func SelectPane(paneID string) error {
	return exec.Command("tmux", "select-pane", "-t", paneID).Run()
}

// SetPaneTitle sets the title of the pane identified by paneID. The title only
// renders when the window's pane-border-status option is "top" or "bottom".
func SetPaneTitle(paneID, title string) error {
	return exec.Command("tmux", "select-pane", "-t", paneID, "-T", title).Run()
}

// EnablePaneBorderStatus turns on pane borders with titles for the window
// containing paneID (-w resolves a pane target to its window).
func EnablePaneBorderStatus(paneID string) error {
	return exec.Command("tmux", "set-option", "-w", "-t", paneID,
		"pane-border-status", "top").Run()
}

// paneIDOf runs a tmux command that prints a pane-id and returns it trimmed.
func paneIDOf(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SendKeys sends keystrokes to a tmux target (e.g. "front:1" or a pane-id "%3").
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

// PopupAnchor controls where a popup is positioned on screen.
type PopupAnchor int

const (
	PopupTopLeft PopupAnchor = iota // flush top-left, row 0 stays visible
	PopupCenter                     // centered on screen
)

// DisplayPopup opens a tmux popup at the given anchor position.
func DisplayPopup(anchor PopupAnchor, title string, width, height int, style, command string) error {
	clientW, clientH, _ := ClientSize()

	var x, y int
	switch anchor {
	case PopupTopLeft:
		x = 0
		y = height + 1
	case PopupCenter:
		x = (clientW - width) / 2
		y = (clientH + height) / 2
	}

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

// SessionSize returns the columns and rows a new detached session should be
// created at so that pane split percentages match the terminal it ends up on.
// Inside tmux it uses the current client (we'll switch-client into the new
// session); outside tmux it measures stdout directly (we'll attach into it).
// Returns (0, 0) when the size can't be determined, leaving tmux's default.
func SessionSize() (cols, rows int) {
	if InTmux() {
		if w, h, err := ClientSize(); err == nil && w > 0 && h > 0 {
			return w, h
		}
	}
	return terminalSize()
}

// terminalSize asks the kernel for the size of the terminal on stdout via the
// TIOCGWINSZ ioctl. Returns (0, 0) when stdout isn't a terminal (e.g. a pipe).
func terminalSize() (cols, rows int) {
	var ws struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0, 0
	}
	return int(ws.Col), int(ws.Row)
}

// KillSession kills a single tmux session by name.
func KillSession(name string) error {
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

// ClientTTY returns the TTY of the current tmux client (e.g. "/dev/ttys001").
func ClientTTY() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_tty}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ClientPID returns the PID of the process that launched the tmux client
// (typically the parent shell).
func ClientPID() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_pid}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DetachClient detaches the current tmux client, returning control to the
// parent terminal.
func DetachClient() error {
	return exec.Command("tmux", "detach-client").Run()
}

// KillServer kills the tmux server, terminating all sessions.
func KillServer() error {
	return exec.Command("tmux", "kill-server").Run()
}
