package tmux

import "testing"

func TestBorderTint(t *testing.T) {
	opts := borderTint("fg=colour8", "", "red")
	want := []windowOption{
		{"pane-border-style", "fg=colour8,fg=red"},
		{"pane-active-border-style", "fg=red"},
		{"pane-border-lines", "heavy"},
	}
	if len(opts) != len(want) {
		t.Fatalf("got %d options, want %d", len(opts), len(want))
	}
	for i := range want {
		if opts[i] != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, opts[i], want[i])
		}
	}
}

func TestHookCommand(t *testing.T) {
	got := hookCommand([]windowOption{
		{"pane-border-style", "#{?pane_in_mode,fg=yellow,fg=green},fg=red"},
		{"pane-border-lines", "heavy"},
	})
	want := `set-option -w pane-border-style "#{?pane_in_mode,fg=yellow,fg=green},fg=red" ; set-option -w pane-border-lines "heavy"`
	if got != want {
		t.Errorf("hookCommand =\n  %s\nwant\n  %s", got, want)
	}
}
