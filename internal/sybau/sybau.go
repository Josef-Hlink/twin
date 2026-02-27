package sybau

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jdham/twin/internal/tmux"
)

// Run launches a tmux popup containing the fzf session picker.
func Run() error {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("no tmux sessions running")
		return nil
	}

	height := len(sessions) + 4

	// Width = longest session name + 5 (border + padding), minimum so "sybau" title fits.
	width := len("sybau") + 5
	for _, s := range sessions {
		if w := len(s) + 5; w > width {
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
	if len(sessions) == 0 {
		fmt.Println("no tmux sessions running")
		return nil
	}

	selected, err := fzfSelect(sessions)
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil // user cancelled
	}

	return tmux.SwitchClient(selected)
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
