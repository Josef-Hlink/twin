package config

import "strings"

// Color is a terminal color reference. The string value is the user-facing
// form: a named color ("red", "magenta"), a 256-color index ("214" or
// "colour214"), or a hex literal ("#ff8800"). Tmux() and Fzf() return the
// representation each tool expects, since they spell digit-only colors
// differently (tmux: "colour214", fzf: "214").
type Color string

// Tmux returns the tmux-style color fragment (e.g. "magenta" or "colour214").
func (c Color) Tmux() string {
	s := string(c)
	if isAllDigits(s) {
		return "colour" + s
	}
	return s
}

// Fzf returns the fzf-style color fragment (e.g. "magenta" or "214").
func (c Color) Fzf() string {
	return strings.TrimPrefix(string(c), "colour")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
