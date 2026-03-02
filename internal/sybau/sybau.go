package sybau

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

// PopupDims computes popup width and height from content metrics.
// When preview is false, the preview columns are ignored and the popup
// is sized for the list alone.
func PopupDims(sessionCount, maxListLine, maxPreviewLine, maxWindowCount int, preview bool) (width, height int) {
	if preview {
		previewCols := maxPreviewLine + 2
		width = maxListLine + 4 + previewCols + 2
		height = max(sessionCount, min(maxWindowCount, 10)) + 4
	} else {
		width = maxListLine + 4 + 1
		height = sessionCount + 4
	}
	return width, height
}

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
	maxPreviewLine := 0
	maxWindowCount := 0
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
		if preview {
			if windows, err := tmux.ListWindows(s); err == nil {
				if len(windows) > maxWindowCount {
					maxWindowCount = len(windows)
				}
				for _, w := range windows {
					if len(w) > maxPreviewLine {
						maxPreviewLine = len(w)
					}
				}
			}
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

	width, height := PopupDims(count, maxListLine, maxPreviewLine, maxWindowCount, preview)

	cmd := self + " sybau-picker"
	if preview {
		cmd += " --preview"
	}
	return tmux.DisplayPopup("sybau", width, height, "fg=magenta bold", cmd)
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

	// Compute preview column width from actual window data so the preview
	// pane is no larger than it needs to be.
	previewCols := 0
	if preview {
		maxPreviewLine := 0
		for _, s := range sessions {
			if s == current {
				continue
			}
			if windows, err := tmux.ListWindows(s); err == nil {
				for _, w := range windows {
					if len(w) > maxPreviewLine {
						maxPreviewLine = len(w)
					}
				}
			}
		}
		previewCols = maxPreviewLine
	}

	selected, err := fzfSelect(lines, previewCols)
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
// When previewCols > 0, a preview pane shows the windows of the highlighted session.
// When previewCols == 0, fzf runs without a preview pane.
func fzfSelect(items []string, previewCols int) (string, error) {
	var fzfArgs []string
	if previewCols > 0 {
		// The sed strips the "[N] " prefix when session numbers are shown,
		// and is a no-op otherwise. The awk colorizes the active window (*) in
		// bold cyan using the window_active flag as a prefix to key off of.
		previewCmd := `tmux list-windows -t "$(echo {} | sed 's/^\[.*\] //')" -F '#{window_active} #{window_index}:#{window_name}#{window_flags}' | awk '{if($1=="1"){printf "\033[1;36m%s\033[0m\n",substr($0,3)}else{print substr($0,3)}}'`
		fzfArgs = append(fzfArgs,
			"--preview", previewCmd,
			"--preview-window", fmt.Sprintf("right:%d:wrap:border-left", previewCols),
		)
	}

	cmd := exec.Command("fzf", fzfArgs...)
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
