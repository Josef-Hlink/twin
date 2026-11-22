package osrs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josef-Hlink/twin/internal/config"
)

func TestSessionURL(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"twin": "url = \"http://localhost:3000\"\nstart-directory = \"/tmp\"\n",
		"dots": "start-directory = \"/tmp\"\n",
		"klym": "start-directory = [",
	} {
		if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const template = "https://{recipe}.{name}.example.com"

	tests := []struct {
		name, session, template, want string
		wantErr                       bool
	}{
		{name: "recipe", session: "twin", template: template, want: "http://localhost:3000"},
		{name: "recipe ignores the template", session: "twin", template: "broken", want: "http://localhost:3000"},
		{name: "ringer", session: "twin@red", template: template, want: "https://twin.red.example.com"},
		{name: "ringer without a template", session: "twin@red"},
		{name: "recipe without a url", session: "dots", template: template},
		{name: "ringer of a recipe without a url", session: "dots@red", template: template},
		{name: "session without a recipe", session: "scratch", template: template},
		{name: "broken recipe", session: "klym", wantErr: true},
		{name: "template without a name", session: "twin@red", template: "broken", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{RecipeDir: dir, Ringers: config.Ringers{URL: tt.template}}
			got, err := sessionURL(cfg, tt.session)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("sessionURL(%q) = %q, %v", tt.session, got, err)
			}
		})
	}
}

func TestOpenerCommand(t *testing.T) {
	for _, tt := range []struct{ configured, goos, want string }{
		{"", "linux", `xdg-open "$(cat)"`},
		{"", "darwin", `open "$(cat)"`},
		{"my-opener --new-tab", "darwin", "my-opener --new-tab"},
	} {
		if got := openerCommand(tt.configured, tt.goos); got != tt.want {
			t.Errorf("openerCommand(%q, %q) = %q, want %q", tt.configured, tt.goos, got, tt.want)
		}
	}
}

// The defaults, with stand-ins for xdg-open and open on PATH: each must get
// the URL as one untouched argument.
func TestDefaultOpeners(t *testing.T) {
	const url = "http://localhost:3000/a b?x=1&y=$(touch pwned);'*#%20"
	for goos, bin := range map[string]string{"linux": "xdg-open", "darwin": "open"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			got := filepath.Join(dir, "got")
			script := "#!/bin/sh\nprintf '%s|%s' \"$#\" \"$1\" > '" + got + "'\n"
			if err := os.WriteFile(filepath.Join(dir, bin), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

			if err := pipeTo(openerCommand("", goos), url); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(got); err != nil || string(b) != "1|"+url {
				t.Fatalf("opener got %q, %v", b, err)
			}
		})
	}
}

func TestPipeToFailure(t *testing.T) {
	for cmdline, want := range map[string]string{
		"echo first >&2; echo no browser >&2; exit 1": "opener failed: no browser",
		"exit 3": "opener failed: exit status 3",
	} {
		if err := pipeTo(cmdline, "http://localhost"); err == nil || err.Error() != want {
			t.Errorf("pipeTo(%q) = %v, want %q", cmdline, err, want)
		}
	}
}

func TestPipeToDoesNotWaitForTheBrowser(t *testing.T) {
	start := time.Now()
	if err := pipeTo("cat >/dev/null; sleep 3 &", "http://localhost"); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("waited %v for a background child", took)
	}
}
