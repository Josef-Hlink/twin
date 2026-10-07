package sybau

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/twins"
)

const Usage = `usage: twin sybau [--preview]

fzf-based session switcher in a tmux popup.
  --preview     show each session's windows in a right-side preview pane
`

// Run launches a tmux popup containing the fzf session picker.
func Run(args []string) error {
	preview := slices.Contains(args, "--preview")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	current, _ := tmux.CurrentSession()
	numbered := cfg.IsOrderedSessions()

	// Calculate popup dimensions from actual content, skipping the current session.
	lines := buildLines(sessions, current, numbered)
	count := len(lines)
	maxListLine := len("sybau") // minimum width
	for _, line := range lines {
		maxListLine = max(maxListLine, len(line))
	}

	if count == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	var maxPreviewLine, maxWindowCount int
	if preview {
		maxPreviewLine, maxWindowCount = windowMetrics(sessions, current)
	}

	width, height := popup.Dims(count, maxListLine, cfg.OptionCap(), maxPreviewLine, maxWindowCount, preview)

	cmd := "sybau-picker"
	if preview {
		cmd += " --preview"
	}
	return popup.Launch("sybau", width, height, cmd, cfg.BorderColor("sybau"))
}

// RunPicker lists tmux sessions, lets the user pick one via fzf, and switches to it.
// It's meant to run inside a tmux popup spawned by Run.
func RunPicker(args []string) error {
	preview := slices.Contains(args, "--preview")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	current, _ := tmux.CurrentSession()
	numbered := cfg.IsOrderedSessions()

	lines := buildLines(sessions, current, numbered)
	if len(lines) == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	previewCols := 0
	var previewCmd string
	if preview {
		previewCols, _ = windowMetrics(sessions, current)
		// The sed strips the "[N] " prefix when session numbers are shown,
		// and a twin's indent. The awk colorizes the active window (*) in
		// bold cyan using the window_active flag as a prefix to key off of.
		previewCmd = `tmux list-windows -t "$(echo {} | sed 's/^\[.*\] //; s/^ *//')" -F '#{window_active} #{window_index}:#{window_name}#{window_flags}' | awk '{if($1=="1"){printf "\033[1;36m%s\033[0m\n",substr($0,3)}else{print substr($0,3)}}'`
	}

	selected, err := popup.FzfSelect(lines, previewCols, previewCmd, cfg.BorderColor("sybau"))
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil // user cancelled
	}

	return tmux.SwitchClient(sessionOf(selected))
}

// buildLines creates the picker lines, skipping the current session. Each
// twin (recipe@name) is listed right under its recipe's session, indented when
// that line is shown; the other sessions keep their creation order and, when
// numbered, get "[N] " from their position among the non-twin sessions.
func buildLines(sessions []string, current string, numbered bool) []string {
	var lines []string
	n := 0
	shown, indent := "", "" // the recipe line twins can nest under
	for _, s := range group(sessions) {
		if recipe, _, isTwin := twins.SplitSession(s); isTwin {
			if s == current {
				continue
			}
			if recipe == shown {
				lines = append(lines, indent+s)
			} else {
				lines = append(lines, s)
			}
			continue
		}

		prefix := ""
		if numbered {
			prefix = fmt.Sprintf("[%d] ", n)
		}
		n++
		if s == current {
			shown = ""
			continue
		}
		lines = append(lines, prefix+s)
		shown, indent = s, strings.Repeat(" ", max(len(prefix), 2))
	}
	return lines
}

// group orders sessions so that each twin follows its recipe's session. A twin
// whose recipe session isn't running keeps its own place.
func group(sessions []string) []string {
	running := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		running[s] = true
	}

	var out []string
	for _, s := range sessions {
		if recipe, _, ok := twins.SplitSession(s); ok && running[recipe] {
			continue // listed under its recipe instead
		}
		out = append(out, s)
		for _, t := range sessions {
			if recipe, _, ok := twins.SplitSession(t); ok && recipe == s {
				out = append(out, t)
			}
		}
	}
	return out
}

// sessionOf turns a picker line back into a session name, dropping the
// "[N] " prefix or a twin's indent.
func sessionOf(line string) string {
	if _, after, ok := strings.Cut(line, "] "); ok {
		line = after
	}
	return strings.TrimSpace(line)
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
