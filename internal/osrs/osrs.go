// Package osrs opens the current session's dev server: it finds the session's
// URL and pipes it to the opener command. How a link gets to a browser is the
// opener's business.
package osrs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/ringers"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin osrs

open the current session's dev server.
  a recipe's session uses the recipe's url
  a ringer uses [ringers] url, when its recipe has a url
the URL goes to twin.toml's opener on stdin (default: xdg-open, or open on
macOS). without a URL it says so and does nothing.
`

// Run opens the current session's URL. It's made for a key binding, where
// nobody sees stdout or stderr, so whatever goes wrong shows on the status
// line as well.
func Run(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("expected no arguments, got %d", len(args))
	}
	if !tmux.InTmux() {
		return errors.New("only works inside tmux")
	}
	err := openCurrent()
	if err != nil {
		tmux.DisplayMessage("twin osrs: " + err.Error())
	}
	return err
}

func openCurrent() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	session, err := tmux.CurrentSession()
	if err != nil {
		return fmt.Errorf("could not resolve current session: %w", err)
	}
	url, err := sessionURL(cfg, session)
	if err != nil {
		return err
	}
	if url == "" {
		msg := "twin osrs: no URL for " + session
		// Not stdout: run-shell would put that over the pane in view mode.
		fmt.Fprintln(os.Stderr, msg)
		tmux.DisplayMessage(msg)
		return nil
	}
	return pipeTo(openerCommand(cfg.Opener, runtime.GOOS), url)
}

// sessionURL returns the URL behind a session, or "" when it has none. A
// ringer never falls back to its recipe's URL: that's another copy's server.
func sessionURL(cfg config.Config, session string) (string, error) {
	recipeName, name, isRinger := ringers.SplitSession(session)
	recipe, err := config.LoadRecipe(cfg.RecipeDir, recipeName)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil // a session made by hand
	}
	if err != nil {
		return "", err
	}
	if recipe.URL == "" || !isRinger {
		return recipe.URL, nil
	}
	return cfg.Ringers.URLFor(ringers.Vars(recipeName, recipe, name, ""))
}

// openerCommand is the configured opener, or the platform's own. xdg-open and
// open only take the URL as an argument, so the defaults read stdin for them.
func openerCommand(configured, goos string) string {
	switch {
	case configured != "":
		return configured
	case goos == "darwin":
		return `open "$(cat)"`
	default:
		return `xdg-open "$(cat)"`
	}
}

// pipeTo runs cmdline through sh with url on stdin, so the URL is never read
// as shell code. A failure carries the last line the command wrote to stderr.
func pipeTo(cmdline, url string) error {
	// A file, not a buffer: with a buffer, Run waits until everything holding
	// the pipe exits, and that can be the browser the opener started.
	stderr, err := os.CreateTemp("", "twin-osrs-")
	if err != nil {
		return err
	}
	defer os.Remove(stderr.Name())
	defer stderr.Close()

	cmd := exec.Command("sh", "-c", cmdline)
	cmd.Stdin = strings.NewReader(url)
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if runErr == nil {
		return nil
	}

	out, err := os.ReadFile(stderr.Name())
	if err != nil {
		return fmt.Errorf("opener failed: %w", runErr)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if last := lines[len(lines)-1]; last != "" {
		return fmt.Errorf("opener failed: %s", last)
	}
	return fmt.Errorf("opener failed: %w", runErr)
}
