package sybau

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

// Run launches a tmux popup containing the fzf session picker.
func Run(args []string) error {
	preview := slices.Contains(args, "--preview")

	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	current, _ := tmux.CurrentSession()
	numbered := showNumbers()

	// Calculate popup dimensions from actual content, skipping the current session.
	count := 0
	maxListLine := len("sybau") // minimum width
	for i, s := range sessions {
		if s == current {
			continue
		}
		count++
		line := s
		if numbered {
			line = fmt.Sprintf("[%d] %s", i, s)
		}
		if len(line) > maxListLine {
			maxListLine = len(line)
		}
	}

	if count == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	var maxPreviewLine, maxWindowCount int
	if preview {
		maxPreviewLine, maxWindowCount = windowMetrics(sessions, current)
	}

	width, height := popup.Dims(count, maxListLine, maxPreviewLine, maxWindowCount, preview)

	cmd := "sybau-picker"
	if preview {
		cmd += " --preview"
	}
	return popup.Launch("sybau", width, height, cmd)
}

// RunPicker lists tmux sessions, lets the user pick one via fzf, and switches to it.
// It's meant to run inside a tmux popup spawned by Run.
func RunPicker(args []string) error {
	preview := slices.Contains(args, "--preview")

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

	previewCols := 0
	var previewCmd string
	if preview {
		previewCols, _ = windowMetrics(sessions, current)
		// The sed strips the "[N] " prefix when session numbers are shown,
		// and is a no-op otherwise. The awk colorizes the active window (*) in
		// bold cyan using the window_active flag as a prefix to key off of.
		previewCmd = `tmux list-windows -t "$(echo {} | sed 's/^\[.*\] //')" -F '#{window_active} #{window_index}:#{window_name}#{window_flags}' | awk '{if($1=="1"){printf "\033[1;36m%s\033[0m\n",substr($0,3)}else{print substr($0,3)}}'`
	}

	selected, err := popup.FzfSelect(lines, previewCols, previewCmd)
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

// windowMetrics returns the longest window line and max window count across
// the given sessions, skipping the named session.
func windowMetrics(sessions []string, skip string) (maxLine, maxCount int) {
	for _, s := range sessions {
		if s == skip {
			continue
		}
		windows, err := tmux.ListWindows(s)
		if err != nil {
			continue
		}
		if len(windows) > maxCount {
			maxCount = len(windows)
		}
		for _, w := range windows {
			if len(w) > maxLine {
				maxLine = len(w)
			}
		}
	}
	return maxLine, maxCount
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
