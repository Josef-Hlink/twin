package sybau

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/ringers"
	"github.com/Josef-Hlink/twin/internal/tmux"
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

	// Calculate popup dimensions from actual content, skipping the current
	// session. Plain lines: color codes would count toward the width.
	lines := buildLines(sessions, current, numbered, noColor)
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

	lines := buildLines(sessions, current, numbered, cfg.Ringers.Color)
	if len(lines) == 0 {
		fmt.Println("no other tmux sessions running")
		return nil
	}

	previewCols := 0
	var previewCmd string
	if preview {
		previewCols, _ = windowMetrics(sessions, current)
		// The sed strips the "[N] " prefix when session numbers are shown,
		// and a ringer's indent. The awk colorizes the active window (*) in
		// bold cyan using the window_active flag as a prefix to key off of.
		previewCmd = `tmux list-windows -t "$(echo {} | sed 's/` + "\x1b" + `\[[0-9;]*m//g; s/^\[.*\] //; s/^ *//')" -F '#{window_active} #{window_index}:#{window_name}#{window_flags}' | awk '{if($1=="1"){printf "\033[1;36m%s\033[0m\n",substr($0,3)}else{print substr($0,3)}}'`
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
// ringer (recipe@name) is listed right under its recipe's session, indented
// when that line is shown, in its [ringers.colors] color when it has one; the
// other sessions keep their creation order and, when numbered, get "[N] "
// from their position among the non-ringer sessions.
func buildLines(sessions []string, current string, numbered bool, colorOf func(string) config.Color) []string {
	var lines []string
	n := 0
	shown, indent := "", "" // the recipe line ringers can nest under
	for _, s := range group(sessions) {
		if recipe, name, isRinger := ringers.SplitSession(s); isRinger {
			if s == current {
				continue
			}
			shownAs := s
			if sgr := colorOf(name).ANSI(); sgr != "" {
				shownAs = "\x1b[" + sgr + "m" + s + "\x1b[0m"
			}
			if recipe == shown {
				lines = append(lines, indent+shownAs)
			} else {
				lines = append(lines, shownAs)
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

// group orders sessions so that each ringer follows its recipe's session. A
// ringer whose recipe session isn't running keeps its own place.
func group(sessions []string) []string {
	running := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		running[s] = true
	}

	var out []string
	for _, s := range sessions {
		if recipe, _, ok := ringers.SplitSession(s); ok && running[recipe] {
			continue // listed under its recipe instead
		}
		out = append(out, s)
		for _, t := range sessions {
			if recipe, _, ok := ringers.SplitSession(t); ok && recipe == s {
				out = append(out, t)
			}
		}
	}
	return out
}

func noColor(string) config.Color { return "" }

// ansiCodes matches the SGR color codes ringer lines are wrapped in.
var ansiCodes = regexp.MustCompile("\x1b\\[[0-9;]*m")

// sessionOf turns a picker line back into a session name, dropping any color
// codes, the "[N] " prefix or a ringer's indent.
func sessionOf(line string) string {
	line = ansiCodes.ReplaceAllString(line, "")
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
