package pbqt

import (
	"fmt"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/tysm"
)

const Usage = `usage: twin pbqt [name]

kill a tmux session.
  (no args)     fzf popup picker of running sessions
  <name>        kill the named session directly
killing the current session switches away first, or falls back to tysm
if it's the last session.
`

// Run kills a tmux session by name or launches a popup picker.
func Run(args []string) error {
	if len(args) > 0 {
		return kill(args[0])
	}
	return pick()
}

// RunPicker runs inside the tmux popup spawned by pick.
// It lists sessions, lets the user pick one via fzf, and kills it.
func RunPicker() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("no tmux sessions running")
		return nil
	}

	current, _ := tmux.CurrentSession()
	numbered := cfg.IsOrderedSessions()
	lines := buildLines(sessions, current, numbered)

	selected, err := popup.FzfSelect(lines, 0, "", cfg.BorderColor("pbqt"))
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil
	}

	name := stripDecorations(selected, numbered)
	return kill(name)
}

// pick shows a picker of running sessions.
// Inside tmux it uses a popup; outside it falls back to inline fzf.
func pick() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("no tmux sessions running")
		return nil
	}

	color := cfg.BorderColor("pbqt")
	numbered := cfg.IsOrderedSessions()
	if tmux.InTmux() {
		return pickPopup(sessions, color, numbered)
	}
	return pickInline(sessions, color, numbered)
}

// pickPopup launches a tmux popup with the pbqt-picker subcommand.
func pickPopup(sessions []string, color config.Color, numbered bool) error {
	current, _ := tmux.CurrentSession()
	lines := buildLines(sessions, current, numbered)

	maxLine := len("pbqt") // minimum width
	for _, line := range lines {
		if len(line) > maxLine {
			maxLine = len(line)
		}
	}

	width, height := popup.Dims(len(lines), maxLine, 0, 0, false)
	return popup.Launch("pbqt", width, height, "pbqt-picker", color)
}

// pickInline shows an inline fzf picker (fallback for outside tmux).
func pickInline(sessions []string, color config.Color, numbered bool) error {
	lines := buildLines(sessions, "", numbered)

	selected, err := popup.FzfSelect(lines, 0, "", color)
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil
	}

	name := stripDecorations(selected, numbered)
	return kill(name)
}

// kill terminates a tmux session, handling the current-session case gracefully.
func kill(name string) error {
	if !tmux.HasSession(name) {
		return fmt.Errorf("session %q does not exist", name)
	}

	current, _ := tmux.CurrentSession()

	if name == current {
		sessions, err := tmux.ListSessions()
		if err != nil {
			return fmt.Errorf("listing sessions: %w", err)
		}

		// Last session — full teardown via tysm.
		if len(sessions) == 1 {
			return tysm.Run([]string{})
		}

		// Pick another session to switch to before killing.
		for _, s := range sessions {
			if s != name {
				if err := tmux.SwitchClient(s); err != nil {
					return fmt.Errorf("switching away from %s: %w", name, err)
				}
				break
			}
		}
	}

	return tmux.KillSession(name)
}

// buildLines creates display lines for the picker, marking the current
// session with " *" and optionally prefixing with "[N] " numbers.
func buildLines(sessions []string, current string, numbered bool) []string {
	lines := make([]string, 0, len(sessions))
	for i, s := range sessions {
		line := s
		if s == current {
			line += " *"
		}
		if numbered {
			line = fmt.Sprintf("[%d] %s", i, line)
		}
		lines = append(lines, line)
	}
	return lines
}

// stripDecorations removes the "[N] " prefix and " *" suffix from a picker line,
// returning the bare session name.
func stripDecorations(selected string, numbered bool) string {
	if numbered {
		if _, after, ok := strings.Cut(selected, "] "); ok {
			selected = after
		}
	}
	selected = strings.TrimSuffix(selected, " *")
	return selected
}

