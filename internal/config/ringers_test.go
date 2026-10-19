package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

const ringersTOML = `
recipe-dir = "/tmp/recipes"

[ringers]
names  = ["red", "blue"]
dir    = "~/copies/{name}/{recipe}"
create = "mkcopy {name} {recipe} {branch}"
remove = "rmcopy {name} {recipe}"
enter  = "copy-shell {name} {dir}"
main   = "copy-main {recipe} {repo}"

[ringers.colors]
red = "214"
`

func TestRingersDecode(t *testing.T) {
	var cfg Config
	if _, err := toml.Decode(ringersTOML, &cfg); err != nil {
		t.Fatal(err)
	}
	r := cfg.Ringers
	if len(r.Names) != 2 || r.Create == "" || r.Remove == "" || r.Enter == "" || r.Main == "" {
		t.Fatalf("decoded %+v", r)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if got := r.Color("red").Tmux(); got != "colour214" {
		t.Fatalf("Color(red) = %q", got)
	}
	if got := r.Color("blue"); got != "" {
		t.Fatalf("Color(blue) = %q, want none", got)
	}
	if !r.Allows("blue") || r.Allows("green") {
		t.Fatalf("Allows() ignores names %v", r.Names)
	}
	if !(Ringers{}).Allows("anything") {
		t.Fatal("without names every name should be allowed")
	}
}

func TestRingersValidate(t *testing.T) {
	tests := []struct {
		name    string
		r       Ringers
		wantErr bool
	}{
		{"unset", Ringers{}, true},
		{"no create", Ringers{Dir: "~/{name}"}, true},
		{"dir without name", Ringers{Dir: "~/copies", Create: "x"}, true},
		{"dir with name twice", Ringers{Dir: "~/{name}/{name}", Create: "x"}, true},
		{"dir with branch", Ringers{Dir: "~/{name}/{branch}", Create: "x"}, true},
		{"ok", Ringers{Dir: "{repo}.copies/{name}", Create: "x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.r.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRingersExpand(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("COPIES", "/srv/copies")
	r := Ringers{
		Dir:    "~/copies/{name}/{recipe}",
		Create: "mkcopy {name} {recipe} {branch} --at {dir}",
		Enter:  "copy-shell {name}",
		Main:   "copy-main {recipe} {repo} {name}{branch}{dir}",
	}
	v := Vars{Recipe: "pylona", Name: "red", Repo: "/home/j/Developer/pro/pylona"}

	if got, want := r.DirFor(v), filepath.Join(home, "copies/red/pylona"); got != want {
		t.Errorf("DirFor() = %q, want %q", got, want)
	}
	if got, want := (Ringers{Dir: "$COPIES/{name}"}).DirFor(v), "/srv/copies/red"; got != want {
		t.Errorf("DirFor() with $VAR = %q, want %q", got, want)
	}

	// No branch: the placeholder vanishes instead of becoming '' (an empty
	// argument), and {dir} is the expanded directory.
	want := "mkcopy red pylona  --at " + filepath.Join(home, "copies/red/pylona")
	if got := r.Command(r.Create, v); got != want {
		t.Errorf("Command(create) = %q, want %q", got, want)
	}

	v.Branch = "feat/42-x"
	if got, want := r.Command("mk {branch}", v), "mk feat/42-x"; got != want {
		t.Errorf("Command() safe branch = %q, want %q", got, want)
	}
	v.Branch = "it's mine"
	if got, want := r.Command("mk {branch}", v), `mk 'it'\''s mine'`; got != want {
		t.Errorf("Command() quoted branch = %q, want %q", got, want)
	}

	// main knows only recipe and repo.
	if got, want := r.MainCommand("pylona", "/home/j/pro pylona"), "copy-main pylona '/home/j/pro pylona' "; got != want {
		t.Errorf("MainCommand() = %q, want %q", got, want)
	}
	if got := (Ringers{}).MainCommand("pylona", "/x"); got != "" {
		t.Errorf("MainCommand() without main = %q, want empty", got)
	}
}

func TestRingersDirPattern(t *testing.T) {
	r := Ringers{Dir: "{repo}.copies/{name}/app"}
	prefix, suffix := r.DirPattern("pylona", "/src/pylona")
	if prefix != "/src/pylona.copies/" || suffix != "/app" {
		t.Fatalf("DirPattern() = %q, %q", prefix, suffix)
	}
}

func TestForRinger(t *testing.T) {
	recipe := Recipe{
		StartDirectory: "/src/pylona",
		Windows: []Window{
			{Command: "nvim"},
			{Command: "pnpm dev", SkipInRingers: true},
			{Panes: []Pane{
				{Command: "lazygit"},
				{Command: "worker", SkipInRingers: true},
			}},
		},
	}
	got := recipe.ForRinger("/copies/red/pylona/")
	if got.StartDirectory != "/copies/red/pylona" {
		t.Errorf("StartDirectory = %q", got.StartDirectory)
	}
	if got.Windows[0].Command != "nvim" || got.Windows[1].Command != "" {
		t.Errorf("window commands = %q, %q", got.Windows[0].Command, got.Windows[1].Command)
	}
	if got.Windows[2].Panes[0].Command != "lazygit" || got.Windows[2].Panes[1].Command != "" {
		t.Errorf("pane commands = %+v", got.Windows[2].Panes)
	}
	// The original recipe is untouched.
	if recipe.Windows[1].Command != "pnpm dev" || recipe.Windows[2].Panes[1].Command != "worker" {
		t.Error("ForRinger modified the original recipe")
	}
}

func TestColorANSI(t *testing.T) {
	for c, want := range map[Color]string{
		"magenta":   "35",
		"colour244": "38;5;244",
		"214":       "38;5;214",
		"#ff8800":   "38;2;255;136;0",
		"nope":      "",
		"":          "",
	} {
		if got := c.ANSI(); got != want {
			t.Errorf("Color(%q).ANSI() = %q, want %q", c, got, want)
		}
	}
}
