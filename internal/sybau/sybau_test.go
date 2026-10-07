package sybau

import (
	"slices"
	"testing"
)

func TestBuildLines(t *testing.T) {
	// Creation order: a twin of dots, then misc, a twin of pylona, and a twin
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
			name:     "numbered: twins indented, numbers skip only the current session",
			current:  "pylona",
			numbered: true,
			want:     []string{"[0] dots", "    dots@x", "pylona@blue", "[2] misc", "krant@old"},
		},
		{
			name:     "current twin is left out",
			current:  "pylona@blue",
			numbered: true,
			want:     []string{"[0] dots", "    dots@x", "[1] pylona", "[2] misc", "krant@old"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLines(sessions, tt.current, tt.numbered)
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
