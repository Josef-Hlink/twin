package wtf

import (
	"errors"
	"fmt"
	"os"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/git"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/twins"
)

const Usage = `usage: twin wtf [<recipe>@]<name>

worktree fold: remove a twin's session and worktree. its branch goes too
when that loses nothing: the upstream is gone (merged PR) or the branch has
no commits beyond origin's default branch.
refuses when the worktree has modified or untracked files, or commits that
are on no branch.
  <recipe>      defaults to the current session's recipe
  <name>        picked in fzf when omitted
`

// Run folds a twin, picking it in a popup when no name is given inside tmux.
func Run(args []string) error {
	recipe, name, err := parse(args)
	if err != nil {
		return err
	}
	if name == "" && tmux.InTmux() {
		return launchPopup(recipe)
	}
	return fold(recipe, name)
}

// RunPicker runs inside the popup spawned by Run, keeping any error on screen.
func RunPicker(args []string) error {
	recipe, name, err := parse(args)
	if err == nil {
		err = fold(recipe, name)
	}
	return popup.Hold("wtf", err)
}

func parse(args []string) (recipe, name string, err error) {
	if len(args) > 1 {
		return "", "", errors.New("expected [<recipe>@]<name>")
	}
	target := ""
	if len(args) == 1 {
		target = args[0]
	}
	return twins.ParseTarget(target)
}

// launchPopup opens a popup running wtf-picker, sized for the recipe's twins.
func launchPopup(recipe string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_, repo, err := twins.Load(cfg, recipe)
	if err != nil {
		return err
	}
	ts, err := repo.List()
	if err != nil {
		return err
	}
	if len(ts) == 0 {
		fmt.Printf("%s has no twins\n", recipe)
		return nil
	}

	maxLine := 0
	for _, t := range ts {
		maxLine = max(maxLine, len(t.Name))
	}
	width, height := popup.Dims(len(ts), maxLine, cfg.OptionCap(), 0, 0, false)
	width, height = max(width, 64), max(height, 10) // room for git's output

	cmd := "wtf-picker " + popup.Quote(twins.SessionName(recipe, ""))
	return popup.Launch("wtf", width, height, cmd, cfg.BorderColor("wtf"))
}

// fold checks that removing the twin loses nothing, then removes its
// worktree, its branch when that's safe, and finally its session.
func fold(recipeName, name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_, repo, err := twins.Load(cfg, recipeName)
	if err != nil {
		return err
	}
	ts, err := repo.List()
	if err != nil {
		return err
	}

	if name == "" {
		if len(ts) == 0 {
			fmt.Printf("%s has no twins\n", recipeName)
			return nil
		}
		name, err = popup.FzfSelect(twins.Names(ts), 0, "", cfg.BorderColor("wtf"))
		if err != nil || name == "" {
			return err // picker error, or cancelled
		}
	}
	t, ok := twins.Find(ts, name)
	if !ok {
		return fmt.Errorf("%s has no twin %q", recipeName, name)
	}
	path := repo.Path(name)
	session := twins.SessionName(recipeName, name)

	// Refuse before touching anything, while the session is still intact.
	if dirty, err := git.IsDirty(path); err != nil {
		return err
	} else if dirty {
		return fmt.Errorf("%s has modified or untracked files; commit, stash or delete them first", path)
	}
	if orphans, err := git.HasOrphanCommits(path); err != nil {
		return err
	} else if orphans {
		return fmt.Errorf("%s is on a detached HEAD with commits no branch has; put them on a branch first", path)
	}

	// Prune so a branch whose PR was merged (and deleted on GitHub) shows as gone.
	if err := git.Fetch(repo.Root, true); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v; branch status may be stale\n", err)
	}

	// The worktree goes before the session: when wtf runs inside the twin's
	// own session, killing it ends wtf too, so it has to be the last step.
	if err := git.RemoveWorktree(repo.Root, path, false); err != nil {
		return err
	}
	os.Remove(repo.Dir()) // only succeeds once the last twin is gone

	summary := "folded " + session
	if msg := dropBranch(repo.Root, t.Branch); msg != "" {
		summary += "; " + msg
	}
	fmt.Println(summary)

	return closeSession(session, recipeName, summary)
}

// dropBranch deletes the twin's branch when no work is lost: its upstream is
// gone (a merged and deleted PR branch; squash merges keep git's own merged
// check from noticing) or it has nothing beyond origin's default branch. It
// returns what it did, for the summary.
func dropBranch(root, branch string) string {
	if branch == "" {
		return "" // detached HEAD, no branch to clean up
	}
	base, err := git.DefaultBranch(root)
	contained := err == nil && git.IsAncestor(root, branch, base)
	if !contained && !git.UpstreamGone(root, branch) {
		return fmt.Sprintf("kept branch %s: it has unmerged commits", branch)
	}
	if err := git.DeleteBranch(root, branch); err != nil {
		return fmt.Sprintf("couldn't delete branch %s: %v", branch, err)
	}
	return "deleted branch " + branch
}

// closeSession kills the twin's session, if it's running. When you're in it,
// you land on the recipe's own session first (or any other one), and the
// summary shows on the status line there.
func closeSession(session, recipeName, summary string) error {
	if !tmux.HasSession(session) {
		return nil
	}
	if !tmux.InTmux() {
		return tmux.KillSession(session)
	}

	current, _ := tmux.CurrentSession()
	if current != session {
		if err := tmux.KillSession(session); err != nil {
			return err
		}
		tmux.DisplayMessage(summary) // a wtf popup closes before you can read it
		return nil
	}

	land := ""
	if tmux.HasSession(recipeName) {
		land = recipeName
	} else if sessions, err := tmux.ListSessions(); err == nil {
		for _, s := range sessions {
			if s != session {
				land = s
				break
			}
		}
	}
	if land == "" {
		return tmux.KillSession(session) // the last session: tmux detaches
	}
	return tmux.SwitchAndKill(land, session, summary)
}
