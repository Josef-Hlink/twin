package icl

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

func noColor(string) config.Color { return "" }

// fakePanes builds panes from "session:window" labels.
func fakePanes(labels ...string) []claudePane {
	panes := make([]claudePane, len(labels))
	for i, l := range labels {
		session, win, _ := strings.Cut(l, ":")
		var w int
		fmt.Sscan(win, &w)
		panes[i] = claudePane{
			pane:  tmux.Pane{SessionName: session, WindowIndex: w},
			label: l,
			state: '-',
		}
	}
	return panes
}

func labels(rows []row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.label)
	}
	return out
}

func TestBuildRows(t *testing.T) {
	cases := []struct {
		name  string
		panes []string
		want  []string
	}{
		{"single windows", []string{"dots:1", "klym@blue:1", "klym:1", "klym@red:1", "twin@green:1"}, []string{
			"dots:1",
			"klym:1",
			"├ @blue:1",
			"└ @red:1",
			"twin", // header: twin's own session runs no claude
			"└ @green:1",
		}},
		{"several windows", []string{"dots:1", "dots:2", "klym:1", "klym:2", "klym@red:1", "klym@red:3"}, []string{
			"dots",
			"├ :1",
			"└ :2",
			"klym",
			"├ :1",
			"├ :2",
			"└ @red",
			"  ├ :1",
			"  └ :3",
		}},
		{"ringer folder mid-list", []string{"twin:1", "twin@blue:1", "twin@blue:2", "twin@red:4"}, []string{
			"twin:1",
			"├ @blue",
			"│ ├ :1",
			"│ └ :2",
			"└ @red:4",
		}},
	}
	for _, c := range cases {
		if got := labels(buildRows(fakePanes(c.panes...), noColor)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRingerColors(t *testing.T) {
	colors := config.Ringers{Colors: map[string]string{"blue": "blue", "red": "colour196"}}
	rows := buildRows(fakePanes("klym:1", "klym@blue:1", "klym@blue:2", "klym@red:1", "klym@green:1"), colors.Color)
	var got []string
	for _, r := range rows {
		got = append(got, r.label+"="+r.color)
	}
	want := []string{
		"klym:1=",
		"├ @blue=34", // the folder and its windows all take blue
		"│ ├ :1=34",
		"│ └ :2=34",
		"├ @red:1=38;5;196",
		"└ @green:1=", // no color configured: stays orange
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNavigation(t *testing.T) {
	// rows: twin (header), @green:1, dots:1, klym:1, @blue:1
	v := newView(fakePanes("twin@green:1", "dots:1", "klym:1", "klym@blue:1"), noColor)
	sel := func() string { return v.rows[v.selected].label }

	steps := []struct {
		do   func()
		want string
	}{
		{func() {}, "└ @green:1"}, // starts past the leading header
		{func() { v.move(-1) }, "└ @blue:1"},
		{func() { v.move(1) }, "└ @green:1"}, // wraps, skipping the header
		{func() { v.move(1) }, "dots:1"},
		{func() { v.moveGroup(1) }, "klym:1"},
		{func() { v.moveGroup(1) }, "└ @green:1"}, // wraps to the first recipe
		{func() { v.moveGroup(-1) }, "klym:1"},
	}
	for i, s := range steps {
		s.do()
		if got := sel(); got != s.want {
			t.Fatalf("step %d: selected %q, want %q", i, got, s.want)
		}
	}
}

func TestScroll(t *testing.T) {
	v := newView(fakePanes("a:1", "b:1", "c@x:1", "d:1"), noColor)
	// rows: a:1, b:1, c (header), @x:1, d:1
	v.selected = 4
	v.scroll(2)
	if v.top != 3 {
		t.Errorf("down to d:1: top %d, want 3", v.top)
	}
	v.selected = 3
	v.scroll(2)
	if v.top != 2 {
		t.Errorf("back up to @x:1: top %d, want 2 (its header in view)", v.top)
	}
}
