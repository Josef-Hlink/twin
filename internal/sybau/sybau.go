package sybau

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

// Run launches a tmux popup containing the fzf session picker.
func Run() error {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	current, _ := tmux.CurrentSession()
	numbered := showNumbers()

	// Calculate popup dimensions, skipping the current session.
	count := 0
	maxWindows := 0
	listWidth := len("sybau") + 5
	for i, s := range sessions {
		if s == current {
			continue
		}
		count++
		line := s
		if numbered {
			line = fmt.Sprintf("[%d] %s", i, s)
		}
		if w := len(line) + 5; w > listWidth {
			listWidth = w
		}
		if wc, err := tmux.WindowCount(s); err == nil && wc > maxWindows {
			maxWindows = wc
		}
	}

	if count == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	// Resolve absolute path so the popup's shell can find the binary.
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	// Preview pane sits to the right of the session list. Cap the preview
	// height so one session with many windows doesn't blow up the popup.
	const previewWidth = 30
	width := listWidth + previewWidth + 3
	previewRows := min(maxWindows, 10)
	height := max(count, previewRows) + 4
	return tmux.DisplayPopup("sybau", width, height, "fg=magenta bold", self+" sybau-picker")
}

// RunPicker lists tmux sessions, lets the user pick one via fzf, and switches to it.
// It's meant to run inside a tmux popup spawned by Run.
func RunPicker() error {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	current, _ := tmux.CurrentSession()
	numbered := showNumbers()

	// Build display lines with original indices, skipping the current session.
	var lines []string
	for i, s := range sessions {
		if s == current {
			continue
		}
		if numbered {
			lines = append(lines, fmt.Sprintf("[%d] %s", i, s))
		} else {
			lines = append(lines, s)
		}
	}

	if len(lines) == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	selected, err := fzfSelect(lines)
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil // user cancelled
	}

	// Strip the "[N] " prefix if present.
	if numbered {
		if _, after, ok := strings.Cut(selected, "] "); ok {
			selected = after
		}
	}

	return tmux.SwitchClient(selected)
}

// showNumbers returns true if session numbers should be displayed in the picker.
// This is tied to the ordered-sessions config option — numbers only make sense
// when sessions have a meaningful order.
func showNumbers() bool {
	cfg, err := config.Load()
	if err != nil {
		return false
	}
	return cfg.IsOrderedSessions()
}

// fzfSelect pipes the given lines to fzf and returns the selected line.
// A preview pane shows the windows of the currently highlighted session.
func fzfSelect(items []string) (string, error) {
	// The sed strips the "[N] " prefix when session numbers are shown,
	// and is a no-op otherwise. The awk colorizes the active window (*) in
	// bold cyan using the window_active flag as a prefix to key off of.
	previewCmd := `tmux list-windows -t "$(echo {} | sed 's/^\[.*\] //')" -F '#{window_active} #{window_index}:#{window_name}#{window_flags}' | awk '{if($1=="1"){printf "\033[1;36m%s\033[0m\n",substr($0,3)}else{print substr($0,3)}}'`
	cmd := exec.Command("fzf",
		"--preview", previewCmd,
		"--preview-window", "right:30:wrap",
	)
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		// fzf exits 130 when the user presses Escape — not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return "", nil
		}
		// fzf exits 1 when there's no match — also not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
