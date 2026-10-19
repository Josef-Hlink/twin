package config

import (
	"fmt"
	"strconv"
	"strings"
)

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

// ANSI returns the SGR parameters that make this color the foreground, e.g.
// "35", "38;5;244" or "38;2;255;136;0", or "" for a name twin doesn't know.
func (c Color) ANSI() string {
	s := strings.TrimPrefix(strings.TrimPrefix(string(c), "colour"), "color")
	if isAllDigits(s) {
		return "38;5;" + s
	}
	if len(s) == 7 && s[0] == '#' {
		rgb, err := strconv.ParseUint(s[1:], 16, 32)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("38;2;%d;%d;%d", rgb>>16, rgb>>8&0xff, rgb&0xff)
	}
	return ansiNames[s]
}

var ansiNames = map[string]string{
	"black": "30", "red": "31", "green": "32", "yellow": "33",
	"blue": "34", "magenta": "35", "cyan": "36", "white": "37",
	"brightblack": "90", "brightred": "91", "brightgreen": "92", "brightyellow": "93",
	"brightblue": "94", "brightmagenta": "95", "brightcyan": "96", "brightwhite": "97",
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
