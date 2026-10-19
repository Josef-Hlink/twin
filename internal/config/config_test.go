package config

import "testing"

func TestOptionCap(t *testing.T) {
	ptr := func(n int) *int { return &n }
	tests := []struct {
		name string
		cfg  Config
		want int
	}{
		{"unset", Config{}, 0},
		{"zero", Config{MaxOptions: ptr(0)}, 0},
		{"negative", Config{MaxOptions: ptr(-3)}, 0},
		{"positive", Config{MaxOptions: ptr(8)}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.OptionCap(); got != tt.want {
				t.Fatalf("OptionCap() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRecipeValidate(t *testing.T) {
	tests := []struct {
		name    string
		recipe  Recipe
		wantErr bool
	}{
		{
			name:   "empty recipe",
			recipe: Recipe{},
		},
		{
			name: "window with commands only",
			recipe: Recipe{Windows: []Window{
				{Commands: []string{"nvim"}},
			}},
		},
		{
			name: "window with command only",
			recipe: Recipe{Windows: []Window{
				{Command: "nvim"},
			}},
		},
		{
			name: "pane with command only",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{{Command: "nvim"}}},
			}},
		},
		{
			name: "window with command and commands",
			recipe: Recipe{Windows: []Window{
				{Command: "nvim", Commands: []string{"ls"}},
			}},
			wantErr: true,
		},
		{
			name: "window with command and panes",
			recipe: Recipe{Windows: []Window{
				{Command: "nvim", Panes: []Pane{{Command: "ls"}}},
			}},
			wantErr: true,
		},
		{
			name: "pane with command and commands",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{{Command: "nvim", Commands: []string{"ls"}}}},
			}},
			wantErr: true,
		},
		{
			name: "valid pane tree",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "right", Size: "30%", Commands: []string{"lazygit"}},
					{Split: "down", Size: "50%", Focus: true},
				}},
			}},
		},
		{
			name: "split-from referencing earlier pane",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "right"},
					{SplitFrom: 1, Split: "down"},
				}},
			}},
		},
		{
			name: "commands and panes both set",
			recipe: Recipe{Windows: []Window{
				{Commands: []string{"nvim"}, Panes: []Pane{{Commands: []string{"ls"}}}},
			}},
			wantErr: true,
		},
		{
			name: "base pane sets split",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{{Split: "right"}}},
			}},
			wantErr: true,
		},
		{
			name: "split-from out of range",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{SplitFrom: 5},
				}},
			}},
			wantErr: true,
		},
		{
			name: "invalid split direction",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "sideways"},
				}},
			}},
			wantErr: true,
		},
		{
			name: "invalid size",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "right", Size: "30"},
				}},
			}},
			wantErr: true,
		},
		{
			name: "two focused panes",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}, Focus: true},
					{Split: "right", Focus: true},
				}},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.recipe.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCmds(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		commands []string
		want     []string
	}{
		{"command only", "nvim", nil, []string{"nvim"}},
		{"commands only", "", []string{"cd src", "make"}, []string{"cd src", "make"}},
		{"command wins over commands", "nvim", []string{"ls"}, []string{"nvim"}},
		{"neither", "", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Window{Command: tt.command, Commands: tt.commands}
			p := Pane{Command: tt.command, Commands: tt.commands}
			for _, got := range [][]string{w.Cmds(), p.Cmds()} {
				if !equalStrings(got, tt.want) {
					t.Fatalf("Cmds() = %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
