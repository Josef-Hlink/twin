package wth

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/git"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/twins"
)

const Usage = `usage: twin wth [<recipe>@]<name> [branch] [--no-attach]

worktree hatch: open a twin, a git worktree of a recipe's repo with its own
session <recipe>@<name>. an existing twin just gets its session (re)opened.
  <recipe>      defaults to the current session's recipe
  <name>        the twin's id and folder: <repo>.twins/<name>/
  [branch]      branch to check out; a new one starts from origin's
                default branch (after a fetch)
  --no-attach   don't switch to the session
name and branch are picked in fzf when omitted: pick an existing one, or
type a new one (alt-enter takes the typed text over a fuzzy match).
`

// options holds the parsed command line.
type options struct {
	recipe   string
	name     string // empty: prompt for it
	branch   string // empty: prompt for it (new twins only)
	noAttach bool
}

// Run hatches (or reopens) a twin. When something still has to be picked
// inside tmux, the whole hatch runs in a popup instead, like fr does.
func Run(args []string) error {
	o, err := parse(args)
	if err != nil {
		return err
	}
	if (o.name == "" || o.branch == "") && tmux.InTmux() {
		return launchPopup(o)
	}
	return hatch(o)
}

// RunPicker runs inside the popup spawned by Run, keeping any error on screen.
func RunPicker(args []string) error {
	o, err := parse(args)
	if err == nil {
		err = hatch(o)
	}
	return popup.Hold("wth", err)
}

func parse(args []string) (options, error) {
	var o options
	var pos []string
	for _, a := range args {
		switch {
		case a == "--no-attach":
			o.noAttach = true
		case strings.HasPrefix(a, "-"):
			return o, fmt.Errorf("unknown flag %s", a)
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) > 2 {
		return o, errors.New("expected [<recipe>@]<name> [branch]")
	}

	target := ""
	if len(pos) > 0 {
		target = pos[0]
	}
	if len(pos) > 1 {
		o.branch = pos[1]
	}
	var err error
	o.recipe, o.name, err = twins.ParseTarget(target)
	return o, err
}

// launchPopup opens a popup that runs the hatch through wth-picker, sized for
// the lists it may show. The recipe is passed explicitly so the popup doesn't
// have to work out the current session again.
func launchPopup(o options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_, repo, err := twins.Load(cfg, o.recipe)
	if err != nil {
		return err
	}
	ts, err := repo.List()
	if err != nil {
		return err
	}

	// An existing twin needs no branch, so there's nothing to pick.
	if _, ok := twins.Find(ts, o.name); ok {
		return hatch(o)
	}

	var items []string
	if o.name == "" {
		items = append(items, twins.Names(ts)...)
	}
	if o.branch == "" {
		branches, _ := git.Branches(repo.Root)
		items = append(items, branches...)
	}
	maxLine := 0
	for _, it := range items {
		maxLine = max(maxLine, len(it))
	}
	width, height := popup.Dims(len(items), maxLine, cfg.OptionCap(), 0, 0, false)
	// Leave room for the fzf header line and for git/setup output.
	width, height = max(width, 64), max(height+1, 14)

	cmd := "wth-picker " + popup.Quote(twins.SessionName(o.recipe, o.name))
	if o.branch != "" {
		cmd += " " + popup.Quote(o.branch)
	}
	if o.noAttach {
		cmd += " --no-attach"
	}
	return popup.Launch("wth", width, height, cmd, cfg.BorderColor("wth"))
}

// hatch prompts for whatever is missing, creates the twin if it doesn't exist
// yet, and opens its session.
func hatch(o options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	recipe, repo, err := twins.Load(cfg, o.recipe)
	if err != nil {
		return err
	}
	ts, err := repo.List()
	if err != nil {
		return err
	}
	color := cfg.BorderColor("wth")

	if o.name == "" {
		o.name, err = popup.FzfPickOrType(twins.Names(ts), "twin> ", color)
		if err != nil || o.name == "" {
			return err // picker error, or cancelled
		}
	}
	if err := twins.ValidateName(o.name); err != nil {
		return err
	}

	// An existing twin only needs its session.
	if t, ok := twins.Find(ts, o.name); ok {
		if o.branch != "" && o.branch != t.Branch {
			return fmt.Errorf("twin %q already exists, on %s", o.name, describeBranch(t.Branch))
		}
		return twins.Open(o.recipe, recipe, repo, o.name, o.noAttach)
	}

	// Check the bootstrap config before creating anything.
	copies := slices.Concat(cfg.Worktree.Copy, recipe.Worktree.Copy)
	for _, c := range copies {
		if !filepath.IsLocal(c) {
			return fmt.Errorf("[worktree] copy entry %q must be a path inside the repo", c)
		}
	}
	setup := slices.Concat(cfg.Worktree.Setup, recipe.Worktree.Setup)

	fmt.Println("fetching…")
	if err := git.Fetch(repo.Root, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v; going on with the refs already here\n", err)
	}

	if o.branch == "" {
		o.branch, err = popup.FzfPickOrType(freeBranches(repo.Root), "branch> ", color)
		if err != nil || o.branch == "" {
			return err
		}
	}

	if err := create(repo, o.name, o.branch, copies, setup); err != nil {
		return err
	}
	return twins.Open(o.recipe, recipe, repo, o.name, o.noAttach)
}

// freeBranches lists the branches a new twin could check out: everything but
// the ones already checked out in a worktree, which git won't check out twice.
func freeBranches(root string) []string {
	branches, _ := git.Branches(root)
	wts, _ := git.Worktrees(root)
	return slices.DeleteFunc(branches, func(b string) bool {
		return slices.ContainsFunc(wts, func(wt git.Worktree) bool { return wt.Branch == b })
	})
}

// create adds the twin's worktree and bootstraps it, rolling everything back
// if bootstrapping fails so a retry starts clean.
func create(repo twins.Repo, name, branch string, copies, setup []string) error {
	path := repo.Path(name)
	git.PruneWorktrees(repo.Root) // frees the path if a twin folder was deleted by hand

	newBranch := !git.LocalBranchExists(repo.Root, branch) && !git.RemoteBranchExists(repo.Root, branch)
	if newBranch {
		if err := git.CheckBranchName(repo.Root, branch); err != nil {
			return err
		}
		base, err := git.DefaultBranch(repo.Root)
		if err != nil {
			return err
		}
		if err := git.AddWorktreeNewBranch(repo.Root, path, branch, base); err != nil {
			return err
		}
	} else if err := git.AddWorktree(repo.Root, path, branch); err != nil {
		return err
	}

	if err := bootstrap(repo.Root, path, copies, setup); err != nil {
		fmt.Fprintf(os.Stderr, "%v; rolling back\n", err)
		if err := git.RemoveWorktree(repo.Root, path, true); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
		if newBranch {
			if err := git.DeleteBranch(repo.Root, branch); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		}
		os.Remove(repo.Dir()) // only succeeds when no other twin is left
		return fmt.Errorf("hatch rolled back: %w", err)
	}
	return nil
}

// bootstrap brings a fresh worktree up to speed: copies untracked files over
// from the main checkout, allows its .envrc, and runs the setup commands.
func bootstrap(root, path string, copies, setup []string) error {
	// Ctrl-C during a slow setup should stop that command, not twin itself,
	// so the rollback still runs. Catching SIGINT does that; the setup
	// commands still get the default Ctrl-C behavior.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	defer signal.Reset(os.Interrupt)

	var copied []string
	for _, c := range copies {
		src, dst := filepath.Join(root, c), filepath.Join(path, c)
		info, err := os.Lstat(src)
		if err != nil {
			continue // not in the main checkout: nothing to copy
		}
		if _, err := os.Lstat(dst); err == nil {
			continue // tracked by git, so the worktree already has it
		}
		if err := copyPath(src, dst, info); err != nil {
			return fmt.Errorf("copying %s: %w", c, err)
		}
		copied = append(copied, c)
	}
	if len(copied) > 0 {
		fmt.Printf("copied: %s\n", strings.Join(copied, ", "))
	}

	// A fresh .envrc is blocked until allowed, at its new path.
	direnv := false
	if _, err := os.Stat(filepath.Join(path, ".envrc")); err == nil {
		if _, err := exec.LookPath("direnv"); err == nil {
			if err := runLoud("", "direnv", "allow", path); err != nil {
				return fmt.Errorf("direnv allow: %w", err)
			}
			// Load the environment once now. Otherwise every pane builds it
			// at the same moment, and parallel nix-direnv evaluations trip
			// over each other's eval cache.
			if err := runLoud(path, "direnv", "exec", path, "true"); err != nil {
				return fmt.Errorf("loading .envrc: %w", err)
			}
			direnv = true
		}
	}

	for _, s := range setup {
		fmt.Printf("$ %s\n", s)
		// With direnv, setup runs inside the environment the panes will get
		// (a nix devShell's pnpm, say). Without it, a plain sh.
		args := []string{"sh", "-c", s}
		if direnv {
			args = append([]string{"direnv", "exec", path}, args...)
		}
		if err := runLoud(path, args...); err != nil {
			return fmt.Errorf("setup %q failed (%w)", s, err)
		}
	}
	return nil
}

// copyPath copies a file, symlink or whole directory from src to dst,
// creating dst's parent directories as needed.
func copyPath(src, dst string, info fs.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	switch {
	case info.IsDir():
		return os.CopyFS(dst, os.DirFS(src))
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	default:
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, info.Mode().Perm())
	}
}

// runLoud runs a command (args[0] is the program) in dir with the terminal
// attached, so its output, and any prompt it shows, is visible.
func runLoud(dir string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// describeBranch names a twin's branch for messages.
func describeBranch(branch string) string {
	if branch == "" {
		return "a detached HEAD"
	}
	return "branch " + branch
}
