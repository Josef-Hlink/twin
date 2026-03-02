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

	sessions = filterCurrentSession(sessions)

	if len(sessions) == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	numbered := showNumbers()

	height := len(sessions) + 4

	// Width = longest display line + 5 (border + padding), minimum so "sybau" title fits.
	width := len("sybau") + 5
	for i, s := range sessions {
		line := s
		if numbered {
			line = fmt.Sprintf("[%d] %s", i+1, s)
		}
		if w := len(line) + 5; w > width {
			width = w
		}
	}

	// Resolve absolute path so the popup's shell can find the binary.
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	return tmux.DisplayPopup("sybau", width, height, "fg=magenta bold", self+" sybau-picker")
}

// RunPicker lists tmux sessions, lets the user pick one via fzf, and switches to it.
// It's meant to run inside a tmux popup spawned by Run.
func RunPicker() error {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	sessions = filterCurrentSession(sessions)

	if len(sessions) == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	numbered := showNumbers()
	lines := sessions
	if numbered {
		lines = make([]string, len(sessions))
		for i, s := range sessions {
			lines[i] = fmt.Sprintf("[%d] %s", i+1, s)
		}
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

// filterCurrentSession removes the currently attached session from the list.
func filterCurrentSession(sessions []string) []string {
	current, err := tmux.CurrentSession()
	if err != nil {
		return sessions // can't determine current session, return all
	}
	filtered := make([]string, 0, len(sessions))
	for _, s := range sessions {
		if s != current {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// fzfSelect pipes the given lines to fzf and returns the selected line.
func fzfSelect(items []string) (string, error) {
	cmd := exec.Command("fzf")
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
