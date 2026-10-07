package twins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/git"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/tspmo"
)

// Sep joins a recipe and a twin's name into the twin's session name, e.g.
// "pylona@blue".
const Sep = "@"

// SessionName returns the session name of a recipe's twin.
func SessionName(recipe, name string) string {
	return recipe + Sep + name
}

// SplitSession splits a twin's session name into its recipe and name. ok is
// false for an ordinary recipe session.
func SplitSession(session string) (recipe, name string, ok bool) {
	return strings.Cut(session, Sep)
}

// RecipeOf returns the recipe a session was built from: the part before the
// @ for a twin, the session name itself otherwise.
func RecipeOf(session string) string {
	recipe, _, _ := strings.Cut(session, Sep)
	return recipe
}

// ValidateName checks that name works as a folder, a tmux session name and
// a command-line argument.
func ValidateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("a twin needs a name")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("twin name %q can't start with -", name)
	case strings.ContainsAny(name, "/.:@") || strings.ContainsFunc(name, unicode.IsSpace):
		return fmt.Errorf("twin name %q can't contain / . : @ or whitespace", name)
	}
	return nil
}

// ParseTarget splits a "[recipe@]name" argument. Without a recipe, it's the
// current session's recipe (so "blue" in pylona@red means pylona@blue). name
// is empty when it still has to be prompted for.
func ParseTarget(arg string) (recipe, name string, err error) {
	if recipe, name, ok := strings.Cut(arg, Sep); ok {
		if recipe == "" {
			return "", "", fmt.Errorf("missing the recipe before %s in %q", Sep, arg)
		}
		return recipe, name, nil
	}
	if !tmux.InTmux() {
		return "", "", fmt.Errorf("outside tmux, name the recipe too: <recipe>@%s", arg)
	}
	session, err := tmux.CurrentSession()
	if err != nil {
		return "", "", fmt.Errorf("could not resolve current session: %w", err)
	}
	return RecipeOf(session), arg, nil
}

// Repo is the git repository a recipe works in, and where its twins live.
type Repo struct {
	Root   string // top-level directory of the main checkout
	Offset string // the recipe's start-directory relative to Root ("." at the root)
}

// Locate finds the repository a recipe's start-directory is in.
func Locate(startDir string) (Repo, error) {
	// git reports the real path, so resolve symlinks before comparing the two.
	real, err := filepath.EvalSymlinks(startDir)
	if err != nil {
		return Repo{}, err
	}
	root, err := git.Toplevel(real)
	if err != nil {
		return Repo{}, fmt.Errorf("%s isn't in a git repo", startDir)
	}
	offset, err := filepath.Rel(root, real)
	if err != nil {
		return Repo{}, err
	}
	return Repo{Root: root, Offset: offset}, nil
}

// Load reads a recipe and locates its repository.
func Load(cfg config.Config, recipeName string) (config.Recipe, Repo, error) {
	recipe, err := config.LoadRecipe(cfg.RecipeDir, recipeName)
	if err != nil {
		return recipe, Repo{}, err
	}
	repo, err := Locate(recipe.StartDirectory)
	if err != nil {
		return recipe, repo, fmt.Errorf("recipe %s: %w", recipeName, err)
	}
	return recipe, repo, nil
}

// Dir is the folder next to the main checkout that holds its twins.
func (r Repo) Dir() string {
	return r.Root + ".twins"
}

// Path is a twin's worktree directory.
func (r Repo) Path(name string) string {
	return filepath.Join(r.Dir(), name)
}

// StartDir is where a twin's session starts: the twin's copy of the recipe's
// start-directory.
func (r Repo) StartDir(name string) string {
	return filepath.Join(r.Path(name), r.Offset)
}

// Twin is an existing twin of a repository.
type Twin struct {
	Name   string
	Branch string // empty on a detached HEAD
}

// List returns the repository's twins: its worktrees directly inside Dir().
func (r Repo) List() ([]Twin, error) {
	if _, err := os.Stat(r.Dir()); err != nil {
		return nil, nil // no twins folder yet, so no twins
	}
	wts, err := git.Worktrees(r.Root)
	if err != nil {
		return nil, err
	}
	var twins []Twin
	for _, wt := range wts {
		if wt.Prunable || filepath.Dir(wt.Path) != r.Dir() {
			continue
		}
		twins = append(twins, Twin{Name: filepath.Base(wt.Path), Branch: wt.Branch})
	}
	return twins, nil
}

// Find returns the named twin; ok is false when it doesn't exist.
func Find(twins []Twin, name string) (Twin, bool) {
	for _, t := range twins {
		if t.Name == name {
			return t, true
		}
	}
	return Twin{}, false
}

// Names returns the names of twins, in order.
func Names(twins []Twin) []string {
	names := make([]string, len(twins))
	for i, t := range twins {
		names[i] = t.Name
	}
	return names
}

// Open builds a twin's session from its recipe if it isn't running yet, then
// switches to it unless noAttach.
func Open(recipeName string, recipe config.Recipe, repo Repo, name string, noAttach bool) error {
	session := SessionName(recipeName, name)
	if !tmux.HasSession(session) {
		if err := tspmo.CreateSession(session, recipe.ForTwin(repo.StartDir(name))); err != nil {
			return fmt.Errorf("creating session %s: %w", session, err)
		}
		fmt.Printf("created: %s\n", session)
	}
	if noAttach {
		return nil
	}
	return GoTo(session)
}

// GoTo switches to the named session inside tmux, or attaches to it outside.
func GoTo(session string) error {
	if tmux.InTmux() {
		return tmux.SwitchClient(session)
	}
	return tmux.AttachSession(session)
}
