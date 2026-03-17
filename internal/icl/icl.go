package icl

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Josef-Hlink/twin/internal/tmux"
)

// claudePane wraps a tmux pane with cached content and detected state.
type claudePane struct {
	pane    tmux.Pane
	label   string // "session:window"
	content string // captured pane output
	state   byte   // '*' busy, '&' menu, '-' idle
}

// Run spawns a centered tmux popup showing the interactive Claude quickview.
func Run() error {
	panes, err := findClaudePanes()
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		fmt.Println("no claude panes found")
		return nil
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	clientW, clientH, _ := tmux.ClientSize()
	width := clientW * 80 / 100
	if width < 40 {
		width = 40
	}
	height := clientH * 70 / 100
	if height < 10 {
		height = 10
	}

	return tmux.DisplayPopup(tmux.PopupCenter, "icl", width, height, "fg=colour214,bold", self+" icl-view")
}

// RunView renders the interactive tabbed Claude pane viewer. Runs inside the popup.
func RunView() error {
	panes, err := findClaudePanes()
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		fmt.Println("no claude panes found")
		return nil
	}

	// Build claudePane list with captured content and state.
	cPanes := make([]claudePane, len(panes))
	for i, p := range panes {
		label := fmt.Sprintf("%s:%d", p.SessionName, p.WindowIndex)
		content, _ := tmux.CapturePane(p.Target)
		cPanes[i] = claudePane{
			pane:    p,
			label:   label,
			content: content,
			state:   detectState(content),
		}
	}

	// Get popup terminal size via stty.
	width, height := termSize()
	if width == 0 || height == 0 {
		width, height = 80, 24
	}

	// Enter raw mode.
	restore, err := sttyRaw()
	if err != nil {
		return fmt.Errorf("entering raw mode: %w", err)
	}
	defer restore()

	selected := 0
	buf := make([]byte, 3)

	for {
		render(os.Stdout, cPanes, selected, width, height)

		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		switch {
		// Escape key alone
		case n == 1 && buf[0] == 27:
			return nil
		// q to quit
		case n == 1 && buf[0] == 'q':
			return nil
		// Enter: switch to pane
		case n == 1 && buf[0] == 13:
			p := cPanes[selected].pane
			target := fmt.Sprintf("%s:%d", p.SessionName, p.WindowIndex)
			tmux.SwitchClient(p.SessionName)
			tmux.SelectWindow(target)
			return nil
		// h/H or left arrow: previous tab
		case n == 1 && (buf[0] == 'h' || buf[0] == 'H'):
			selected = (selected - 1 + len(cPanes)) % len(cPanes)
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'D':
			selected = (selected - 1 + len(cPanes)) % len(cPanes)
		// l/L or right arrow: next tab
		case n == 1 && (buf[0] == 'l' || buf[0] == 'L'):
			selected = (selected + 1) % len(cPanes)
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'C':
			selected = (selected + 1) % len(cPanes)
		}
	}

	return nil
}

// --- Terminal helpers ---

// sttyRaw puts the terminal in raw mode and returns a function to restore it.
func sttyRaw() (restore func(), err error) {
	// Save current settings. Must connect stdin so stty sees the terminal.
	save := exec.Command("stty", "-g")
	save.Stdin = os.Stdin
	out, err := save.Output()
	if err != nil {
		return nil, err
	}
	saved := strings.TrimSpace(string(out))

	// Enter raw mode.
	raw := exec.Command("stty", "raw", "-echo")
	raw.Stdin = os.Stdin
	if err := raw.Run(); err != nil {
		return nil, err
	}

	return func() {
		cmd := exec.Command("stty", saved)
		cmd.Stdin = os.Stdin
		cmd.Run()
	}, nil
}

// termSize returns terminal cols and rows via stty size.
func termSize() (width, height int) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return 0, 0
	}
	h, _ := strconv.Atoi(fields[0])
	w, _ := strconv.Atoi(fields[1])
	return w, h
}

// --- State detection ---

var menuLineRe = regexp.MustCompile(`^\s*\d+\.\s`)

// detectState scans the last ~20 lines of captured content to determine pane state.
func detectState(content string) byte {
	lines := lastLines(content, 20)

	// Check last 10 lines for menu pattern (2+ matches = menu).
	scanLines := lines
	if len(scanLines) > 10 {
		scanLines = scanLines[len(scanLines)-10:]
	}
	menuCount := 0
	for _, line := range scanLines {
		if isMenuLine(line) {
			menuCount++
		}
	}
	if menuCount >= 2 {
		return '&'
	}

	// Check all 20 lines for busy pattern.
	for i := len(lines) - 1; i >= 0; i-- {
		if isBusyLine(lines[i]) {
			return '*'
		}
	}

	return '-'
}

// isMenuLine checks if a line matches the "N. " pattern (digit, dot, space).
func isMenuLine(line string) bool {
	return menuLineRe.MatchString(line)
}

// isBusyLine checks if a line contains a word ending in "ing…" or "ing..."
// which matches Claude's spinner status text (Crunching…, Reading…, etc.).
func isBusyLine(line string) bool {
	for w := range strings.FieldsSeq(line) {
		lower := strings.ToLower(w)
		if strings.HasSuffix(lower, "ing…") || strings.HasSuffix(lower, "ing...") {
			return true
		}
	}
	return false
}

// --- Rendering ---

const (
	ansiOrange  = "\033[38;5;214m"
	ansiReverse = "\033[7m"
	ansiReset   = "\033[0m"
	ansiClear = "\033[2J\033[H" // clear screen + cursor home
	ansiDim   = "\033[2m"
)

// render draws the full TUI: tab bar, separator, preview, status line.
func render(w io.Writer, panes []claudePane, selected, width, height int) {
	var b strings.Builder

	// Clear screen and move cursor home.
	b.WriteString(ansiClear)

	// --- Tab bar (line 1) ---
	b.WriteString(ansiOrange)
	var tabBar strings.Builder
	for i, p := range panes {
		tab := fmt.Sprintf(" %s %c ", p.label, p.state)
		if i == selected {
			tabBar.WriteString(ansiReverse)
			tabBar.WriteString(tab)
			tabBar.WriteString(ansiReset)
			tabBar.WriteString(ansiOrange)
		} else {
			tabBar.WriteString(tab)
		}
		if i < len(panes)-1 {
			tabBar.WriteString("|")
		}
	}
	// Pad or truncate tab bar to width.
	tabStr := tabBar.String()
	b.WriteString(tabStr)
	b.WriteString(ansiReset)
	b.WriteString("\r\n")

	// --- Separator (line 2) ---
	b.WriteString(ansiOrange)
	b.WriteString(strings.Repeat("━", width))
	b.WriteString(ansiReset)
	b.WriteString("\r\n")

	// --- Preview area (lines 3 to height-1) ---
	previewHeight := max(height-3, 1) // tab bar + separator + status line

	content := panes[selected].content
	previewLines := lastLines(content, previewHeight)

	for i := range previewHeight {
		if i < len(previewLines) {
			line := truncVisible(previewLines[i], width)
			b.WriteString(line)
			b.WriteString(ansiReset) // prevent color bleed across lines
		}
		b.WriteString("\r\n")
	}

	// --- Status line (last line) ---
	b.WriteString(ansiDim)
	status := " H/L: navigate  Enter: switch  q: close"
	b.WriteString(truncVisible(status, width))
	b.WriteString(ansiReset)

	w.Write([]byte(b.String()))
}

// truncVisible truncates a string to n visible columns, skipping ANSI
// escape sequences so they don't count toward the width.
func truncVisible(s string, n int) string {
	visible := 0
	i := 0
	for i < len(s) {
		// Skip ANSI escape sequences (\033[...m).
		if i+1 < len(s) && s[i] == '\033' && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++ // skip the 'm'
			}
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		visible++
		if visible > n {
			return s[:i]
		}
		i += size
	}
	return s
}

// --- Claude pane detection ---

type proc struct {
	ppid int
	args string
}

func findClaudePanes() ([]tmux.Pane, error) {
	all, err := tmux.ListAllPanes()
	if err != nil {
		return nil, fmt.Errorf("listing panes: %w", err)
	}

	procs, err := allProcesses()
	if err != nil {
		// Fallback: just check the pane's foreground command.
		var matched []tmux.Pane
		for _, p := range all {
			if strings.Contains(strings.ToLower(p.Command), "claude") {
				matched = append(matched, p)
			}
		}
		return matched, nil
	}

	// Build children map for tree walking.
	children := make(map[int][]int)
	for pid, p := range procs {
		children[p.ppid] = append(children[p.ppid], pid)
	}

	var matched []tmux.Pane
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Command), "claude") {
			matched = append(matched, p)
			continue
		}
		if hasClaude(p.PanePID, procs, children) {
			matched = append(matched, p)
		}
	}
	return matched, nil
}

func allProcesses() (map[int]proc, error) {
	out, err := exec.Command("ps", "-Ao", "pid=", "-o", "ppid=", "-o", "args=").Output()
	if err != nil {
		return nil, err
	}

	procs := make(map[int]proc)
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		if pid == 0 {
			continue
		}
		procs[pid] = proc{ppid: ppid, args: strings.Join(fields[2:], " ")}
	}
	return procs, nil
}

// hasClaude walks the process tree from rootPID looking for a claude process.
func hasClaude(rootPID int, procs map[int]proc, children map[int][]int) bool {
	queue := children[rootPID]
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if p, ok := procs[pid]; ok {
			if strings.Contains(strings.ToLower(p.args), "claude") {
				return true
			}
			queue = append(queue, children[pid]...)
		}
	}
	return false
}

// --- Text formatting ---

// lastLines trims trailing blank lines and returns the last n lines.
func lastLines(content string, n int) []string {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
