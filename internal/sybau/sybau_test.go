package sybau

import (
	"slices"
	"testing"

	"github.com/Josef-Hlink/twin/internal/config"
)

func TestBuildLines(t *testing.T) {
	// Creation order: a ringer of dots, then misc, a ringer of pylona, and a ringer
	// whose recipe session (krant) isn't running.
	sessions := []string{"dots", "pylona", "dots@x", "misc", "pylona@blue", "krant@old"}

	tests := []struct {
		name     string
		current  string
		numbered bool
		want     []string
	}{
		{
			name:    "grouped, unnumbered",
			current: "misc",
			want:    []string{"dots", "  dots@x", "pylona", "  pylona@blue", "krant@old"},
		},
		{
			name:     "numbered: ringers indented, numbers skip only the current session",
			current:  "pylona",
			numbered: true,
			want:     []string{"[0] dots", "    dots@x", "pylona@blue", "[2] misc", "krant@old"},
		},
		{
			name:     "current ringer is left out",
			current:  "pylona@blue",
			numbered: true,
			want:     []string{"[0] dots", "    dots@x", "[1] pylona", "[2] misc", "krant@old"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLines(sessions, tt.current, tt.numbered, noColor)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("buildLines() =\n%q\nwant\n%q", got, tt.want)
			}
			for _, line := range got {
				if s := sessionOf(line); !slices.Contains(sessions, s) {
					t.Fatalf("sessionOf(%q) = %q, not a session", line, s)
				}
			}
		})
	}
}

func TestBuildLinesColorsRingers(t *testing.T) {
	colors := map[string]config.Color{"blue": "blue", "old": "colour244"}
	colorOf := func(name string) config.Color { return colors[name] }
	got := buildLines([]string{"pylona", "pylona@blue", "misc", "krant@old"}, "misc", false, colorOf)
	want := []string{"pylona", "  \x1b[34mpylona@blue\x1b[0m", "\x1b[38;5;244mkrant@old\x1b[0m"}
	if !slices.Equal(got, want) {
		t.Fatalf("buildLines() =\n%q\nwant\n%q", got, want)
	}
	for i, s := range []string{"pylona", "pylona@blue", "krant@old"} {
		if sessionOf(got[i]) != s {
			t.Fatalf("sessionOf(%q) = %q, want %q", got[i], sessionOf(got[i]), s)
		}
	}
}
