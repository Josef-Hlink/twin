package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// run executes git in dir and returns its trimmed stdout. When git fails, the
// error carries git's own stderr message so callers can show why.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// runLoud executes git in dir with its output going straight to the terminal,
// for steps the user should see happen (fetching, creating a worktree).
func runLoud(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s failed", args[0])
	}
	return nil
}

// ok reports whether a git command exits 0, for yes/no questions like
// "does this ref exist".
func ok(dir string, args ...string) bool {
	return exec.Command("git", append([]string{"-C", dir}, args...)...).Run() == nil
}

// Toplevel returns the root of the working tree containing dir.
func Toplevel(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}

// Fetch fetches the default remote. prune also drops remote-tracking branches
// that were deleted upstream, which is what makes a merged branch show as gone.
func Fetch(dir string, prune bool) error {
	args := []string{"fetch"}
	if prune {
		args = append(args, "--prune")
	}
	return runLoud(dir, args...)
}

// DefaultBranch returns origin's default branch as a remote-tracking ref,
// e.g. "origin/main".
func DefaultBranch(dir string) (string, error) {
	ref, err := run(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", fmt.Errorf("can't find origin's default branch; run `git remote set-head origin --auto` in %s", dir)
	}
	return ref, nil
}

// Branches returns branch names a worktree could check out: local branches,
// then remote-only ones (without their remote prefix), each most recent first.
func Branches(dir string) ([]string, error) {
	local, err := run(dir, "for-each-ref", "--sort=-committerdate", "--format=%(refname:lstrip=2)", "refs/heads")
	if err != nil {
		return nil, err
	}
	remote, err := run(dir, "for-each-ref", "--sort=-committerdate", "--format=%(refname:lstrip=3)", "refs/remotes")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, n := range append(lines(local), lines(remote)...) {
		if n != "HEAD" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	return names, nil
}

// LocalBranchExists reports whether refs/heads/<name> exists.
func LocalBranchExists(dir, name string) bool {
	return ok(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
}

// RemoteBranchExists reports whether any remote has a branch called name.
func RemoteBranchExists(dir, name string) bool {
	out, err := run(dir, "for-each-ref", "--format=%(refname:lstrip=3)", "refs/remotes")
	return err == nil && slices.Contains(lines(out), name)
}

// CheckBranchName returns an error when name isn't a valid new branch name.
func CheckBranchName(dir, name string) error {
	if !ok(dir, "check-ref-format", "--branch", name) {
		return fmt.Errorf("%q isn't a valid branch name", name)
	}
	return nil
}

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path     string
	Branch   string // short branch name; empty when detached
	Prunable bool   // its directory is gone
}

// Worktrees lists the worktrees of the repository containing dir, the main
// checkout first.
func Worktrees(dir string) ([]Worktree, error) {
	out, err := run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	// The porcelain format is one "key value" line per attribute, with a blank
	// line between worktrees.
	var wts []Worktree
	for block := range strings.SplitSeq(out, "\n\n") {
		var wt Worktree
		for _, line := range lines(block) {
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "worktree":
				wt.Path = value
			case "branch":
				wt.Branch = strings.TrimPrefix(value, "refs/heads/")
			case "prunable":
				wt.Prunable = true
			}
		}
		if wt.Path != "" {
			wts = append(wts, wt)
		}
	}
	return wts, nil
}

// AddWorktree checks out an existing branch in a new worktree at path. A
// branch that only exists on a remote gets a local tracking branch.
func AddWorktree(dir, path, branch string) error {
	return runLoud(dir, "worktree", "add", path, branch)
}

// AddWorktreeNewBranch creates branch from base in a new worktree at path.
// --no-track keeps base (e.g. origin/main) from becoming its upstream.
func AddWorktreeNewBranch(dir, path, branch, base string) error {
	return runLoud(dir, "worktree", "add", "--no-track", "-b", branch, path, base)
}

// RemoveWorktree deletes the worktree at path. Without force, git refuses
// when it has modified or untracked files.
func RemoveWorktree(dir, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := run(dir, args...)
	return err
}

// PruneWorktrees forgets worktrees whose directories were deleted by hand, so
// their paths can be used again.
func PruneWorktrees(dir string) error {
	_, err := run(dir, "worktree", "prune")
	return err
}

// IsDirty reports whether the worktree at dir has modified or untracked
// files (ignored files don't count, matching `git worktree remove`).
func IsDirty(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	return out != "", err
}

// HasOrphanCommits reports whether the worktree at dir is on a detached HEAD
// that no branch, tag or remote branch contains. Removing such a worktree
// silently drops those commits.
func HasOrphanCommits(dir string) (bool, error) {
	if ok(dir, "symbolic-ref", "-q", "HEAD") {
		return false, nil // on a branch
	}
	out, err := run(dir, "for-each-ref", "--contains", "HEAD", "--count=1", "refs/heads", "refs/tags", "refs/remotes")
	return out == "", err
}

// UpstreamGone reports whether branch tracks a remote branch that no longer
// exists (after a pruning fetch), which is what a merged PR branch looks like.
func UpstreamGone(dir, branch string) bool {
	out, err := run(dir, "for-each-ref", "--format=%(upstream:track)", "refs/heads/"+branch)
	return err == nil && out == "[gone]"
}

// IsAncestor reports whether commit a is contained in b, i.e. a has no
// commits of its own on top of b.
func IsAncestor(dir, a, b string) bool {
	return ok(dir, "merge-base", "--is-ancestor", a, b)
}

// DeleteBranch force-deletes a local branch. Callers decide when that's safe:
// squash merges make git's own merged check useless.
func DeleteBranch(dir, branch string) error {
	_, err := run(dir, "branch", "-D", branch)
	return err
}

// lines splits command output into non-empty lines.
func lines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
