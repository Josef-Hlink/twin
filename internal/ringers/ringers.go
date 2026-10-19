// Package ringers is the session side of twin's ringers: a ringer is a named
// copy of a recipe's session, <recipe>@<name>, rooted in its own directory.
// What makes that directory a real copy (a clone, a container, a remote
// box...) is up to the commands in twin.toml's [ringers]; this package names
// ringers, finds the existing ones, runs those commands and builds sessions.
package ringers

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/tspmo"
)

// Sep joins a recipe and a ringer's name into the ringer's session name, e.g.
// "pylona@blue".
const Sep = "@"

// SessionName returns the session name of a recipe's ringer.
func SessionName(recipe, name string) string {
	return recipe + Sep + name
}

// SplitSession splits a ringer's session name into its recipe and name. ok is
// false for an ordinary recipe session.
func SplitSession(session string) (recipe, name string, ok bool) {
	return strings.Cut(session, Sep)
}

// RecipeOf returns the recipe a session was built from: the part before the
// @ for a ringer, the session name itself otherwise.
func RecipeOf(session string) string {
	recipe, _, _ := strings.Cut(session, Sep)
	return recipe
}

// ValidateName checks that name works as a tmux session name, inside a path
// and as a command-line argument.
func ValidateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("a ringer needs a name")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("ringer name %q can't start with -", name)
	case strings.ContainsAny(name, "/.:@'\"\\") || strings.ContainsFunc(name, unicode.IsSpace):
		return fmt.Errorf("ringer name %q can't contain / . : @ quotes, \\ or whitespace", name)
	}
	return nil
}

// ParseTarget splits a "[recipe@]name" argument. Without a recipe, it's the
// current session's recipe (so "blue" in pylona@red means pylona@blue). name
// is empty when it still has to be picked.
func ParseTarget(arg string) (recipe, name string, err error) {
	if recipe, name, ok := strings.Cut(arg, Sep); ok {
		if recipe == "" {
			return "", "", fmt.Errorf("missing the recipe before %s in %q", Sep, arg)
		}
		return recipe, name, nil
	}
	if !tmux.InTmux() {
		return "", "", fmt.Errorf("outside tmux, name the recipe too: <recipe>%s%s", Sep, arg)
	}
	session, err := tmux.CurrentSession()
	if err != nil {
		return "", "", fmt.Errorf("could not resolve current session: %w", err)
	}
	return RecipeOf(session), arg, nil
}

// Load reads the config and a recipe, and checks that [ringers] is usable.
func Load(recipeName string) (config.Config, config.Recipe, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, config.Recipe{}, err
	}
	if err := cfg.Ringers.Validate(); err != nil {
		return cfg, config.Recipe{}, err
	}
	recipe, err := config.LoadRecipe(cfg.RecipeDir, recipeName)
	return cfg, recipe, err
}

// Vars are the template values for one ringer of a recipe.
func Vars(recipeName string, recipe config.Recipe, name, branch string) config.Vars {
	return config.Vars{Recipe: recipeName, Name: name, Branch: branch, Repo: recipe.StartDirectory}
}

// Dir is a ringer's directory, where its session starts.
func Dir(cfg config.Config, recipeName string, recipe config.Recipe, name string) string {
	return cfg.Ringers.DirFor(Vars(recipeName, recipe, name, ""))
}

// List returns the names of a recipe's existing ringers: the ones whose
// directory exists. With [ringers] names set, it checks those, in order;
// otherwise it looks for any directory matching the dir template.
func List(cfg config.Config, recipeName string, recipe config.Recipe) []string {
	var names []string
	if len(cfg.Ringers.Names) > 0 {
		for _, n := range cfg.Ringers.Names {
			if isDir(Dir(cfg, recipeName, recipe, n)) {
				names = append(names, n)
			}
		}
		return names
	}

	prefix, suffix := cfg.Ringers.DirPattern(recipeName, recipe.StartDirectory)
	matches, _ := filepath.Glob(prefix + "*" + suffix)
	for _, m := range matches {
		n := strings.TrimSuffix(strings.TrimPrefix(m, prefix), suffix)
		if ValidateName(n) == nil && isDir(m) {
			names = append(names, n)
		}
	}
	return names
}

// Free returns the configured names a recipe has no ringer for yet.
func Free(cfg config.Config, existing []string) []string {
	return slices.DeleteFunc(slices.Clone(cfg.Ringers.Names), func(n string) bool {
		return slices.Contains(existing, n)
	})
}

// Open builds a ringer's session from its recipe if it isn't running yet,
// then switches to it unless noAttach. Every pane starts through [ringers]
// enter, so splits made by hand later land inside the ringer too.
func Open(cfg config.Config, recipeName string, recipe config.Recipe, name string, noAttach bool) error {
	session := SessionName(recipeName, name)
	if !tmux.HasSession(session) {
		v := Vars(recipeName, recipe, name, "")
		opts := tspmo.Options{Tint: cfg.Ringers.Color(name)}
		if cfg.Ringers.Enter != "" {
			opts.Shell = cfg.Ringers.Command(cfg.Ringers.Enter, v)
		}
		dir := cfg.Ringers.DirFor(v)
		if err := tspmo.CreateSession(session, recipe.ForRinger(dir), opts); err != nil {
			return fmt.Errorf("creating session %s: %w", session, err)
		}
		fmt.Printf("created: %s\n", session)
	}
	if noAttach {
		return nil
	}
	return GoTo(session)
}

// Reopen rebuilds the session of an existing ringer from its session name,
// e.g. after a reboot. It never creates a ringer; that's ring's job.
func Reopen(session string, noAttach bool) error {
	recipeName, name, _ := SplitSession(session)
	cfg, recipe, err := Load(recipeName)
	if err != nil {
		return err
	}
	if !slices.Contains(List(cfg, recipeName, recipe), name) {
		return fmt.Errorf("%s has no ringer %q; make one with `twin ring %s`", recipeName, name, session)
	}
	return Open(cfg, recipeName, recipe, name, noAttach)
}

// Dormant returns the session names of ringers that exist but have no session
// running, across the given recipes. Nothing without a usable [ringers].
func Dormant(cfg config.Config, recipes []string) []string {
	if cfg.Ringers.Validate() != nil {
		return nil
	}
	var sessions []string
	for _, recipeName := range recipes {
		recipe, err := config.LoadRecipe(cfg.RecipeDir, recipeName)
		if err != nil {
			continue
		}
		for _, n := range List(cfg, recipeName, recipe) {
			if s := SessionName(recipeName, n); !tmux.HasSession(s) {
				sessions = append(sessions, s)
			}
		}
	}
	return sessions
}

// Run runs a [ringers] command line through sh in dir, with the terminal
// attached so its output (and any prompt) shows. Ctrl-C stops the command,
// not twin, so twin can still report that it failed.
func Run(cmdline, dir string) error {
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	defer signal.Reset(os.Interrupt)

	fmt.Printf("$ %s\n", cmdline)
	cmd := exec.Command("sh", "-c", cmdline)
	if isDir(dir) {
		cmd.Dir = dir
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// GoTo switches to the named session inside tmux, or attaches to it outside.
func GoTo(session string) error {
	if tmux.InTmux() {
		return tmux.SwitchClient(session)
	}
	return tmux.AttachSession(session)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
