package icl

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Josef-Hlink/twin/internal/tmux"
)

const (
	captureLines = 10
	maxCols      = 50
)

// Run spawns a right-side tmux popup showing Claude agent pane contents.
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

	_, clientH, _ := tmux.ClientSize()
	width := maxCols + 4
	contentH := len(panes)*(captureLines+3) + 3
	height := min(contentH, clientH-2)
	height = max(height, 10)

	return tmux.DisplayPopupRight("icl", width, height, "fg=colour214,bold", self+" icl-view")
}

// RunView renders Claude pane summaries and exits. Runs inside the popup.
// The popup closes automatically when this returns (tmux -E flag).
func RunView() error {
	panes, err := findClaudePanes()
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		fmt.Println("no claude panes found")
		return nil
	}

	orange := "\033[38;5;214m"
	reset := "\033[0m"
	for i, p := range panes {
		label := fmt.Sprintf("%s:%d", p.SessionName, p.WindowIndex)
		// ━━ label ━━━━━━━━━━━━━━━━━━━━
		pad := maxCols - len("━━"+" "+label+" ") // leading ━━, spaces around label
		if pad < 2 {
			pad = 2
		}
		fmt.Printf("%s━━ %s %s%s\n", orange, label, strings.Repeat("━", pad), reset)

		content, err := tmux.CapturePane(p.Target)
		if err != nil {
			fmt.Printf("  (error: %v)\n", err)
			continue
		}

		for _, line := range lastLines(content, captureLines) {
			fmt.Println(trunc(line, maxCols))
		}
		if i < len(panes)-1 {
			fmt.Println()
		}
	}

	// Wait for any keypress to dismiss.
	fmt.Print("\npress any key to close")
	b := make([]byte, 1)
	os.Stdin.Read(b)
	return nil
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
	for _, line := range strings.Split(string(out), "\n") {
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

// trunc truncates a string to n visible runes.
func trunc(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}
