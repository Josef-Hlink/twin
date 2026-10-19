package ring

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/ringers"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin ring [<recipe>@]<name> [branch] [--no-attach]

open a ringer: a named copy of a recipe's session, <recipe>@<name>, made
real by the commands in twin.toml's [ringers]. a ringer whose dir exists
just gets its session (re)opened; a new one runs create first.
  <recipe>      defaults to the current session's recipe
  <name>        one of [ringers] names when those are set
  [branch]      handed to create as {branch}
  --no-attach   don't switch to the session
name and branch are picked in fzf when omitted (the branch only when create
uses {branch}): pick one, or type a new one (alt-enter takes the typed text
over a fuzzy match).
`

// options holds the parsed command line.
type options struct {
	recipe   string
	name     string // empty: pick it
	branch   string // empty: pick it, when create wants one
	noAttach bool
}

// Run opens (or creates) a ringer. When something still has to be picked
// inside tmux, the whole thing runs in a popup instead, like fr does, so
// create's output shows there too.
func Run(args []string) error {
	o, err := parse(args)
	if err != nil {
		return err
	}
	if tmux.InTmux() {
		return launchPopup(o)
	}
	return ring(o)
}

// RunPicker runs inside the popup spawned by Run, keeping any error on screen.
func RunPicker(args []string) error {
	o, err := parse(args)
	if err == nil {
		err = ring(o)
	}
	return popup.Hold("ring", err)
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
	o.recipe, o.name, err = ringers.ParseTarget(target)
	return o, err
}

// launchPopup opens a popup that runs ring through ring-picker, sized for the
// lists it may show and for create's output. An existing ringer needs neither,
// so it opens right here.
func launchPopup(o options) error {
	cfg, recipe, err := ringers.Load(o.recipe)
	if err != nil {
		return err
	}
	existing := ringers.List(cfg, o.recipe, recipe)
	if o.name != "" && slices.Contains(existing, o.name) {
		return ring(o)
	}

	var items []string
	if o.name == "" {
		items = append(items, choices(cfg, existing)...)
	}
	if o.branch == "" && wantsBranch(cfg) {
		items = append(items, branches(recipe.StartDirectory)...)
	}
	maxLine := 0
	for _, it := range items {
		maxLine = max(maxLine, len(it))
	}
	width, height := popup.Dims(len(items), maxLine, cfg.OptionCap(), 0, 0, false)
	// Leave room for the fzf header line and for create's output.
	width, height = max(width, 72), max(height+1, 16)

	cmd := "ring-picker " + popup.Quote(ringers.SessionName(o.recipe, o.name))
	if o.branch != "" {
		cmd += " " + popup.Quote(o.branch)
	}
	if o.noAttach {
		cmd += " --no-attach"
	}
	return popup.Launch("ring", width, height, cmd, cfg.BorderColor("ring"))
}

// ring picks whatever is missing, creates the ringer if it doesn't exist yet,
// and opens its session.
func ring(o options) error {
	cfg, recipe, err := ringers.Load(o.recipe)
	if err != nil {
		return err
	}
	existing := ringers.List(cfg, o.recipe, recipe)
	color := cfg.BorderColor("ring")

	if o.name == "" {
		// With fixed names there's nothing new to type; otherwise typing a
		// name that matches nothing makes a new ringer.
		if len(cfg.Ringers.Names) > 0 {
			o.name, err = popup.FzfSelect(choices(cfg, existing), 0, "", color)
		} else {
			o.name, err = popup.FzfPickOrType(existing, "ringer> ", color)
		}
		if err != nil || o.name == "" {
			return err // picker error, or cancelled
		}
		o.name = strings.TrimSuffix(o.name, existingMark)
	}
	if err := ringers.ValidateName(o.name); err != nil {
		return err
	}
	if !cfg.Ringers.Allows(o.name) {
		return fmt.Errorf("%q isn't one of [ringers] names (%s)", o.name, strings.Join(cfg.Ringers.Names, ", "))
	}

	if slices.Contains(existing, o.name) {
		if o.branch != "" {
			fmt.Fprintf(os.Stderr, "%s already exists; ignoring branch %s\n", ringers.SessionName(o.recipe, o.name), o.branch)
		}
		return ringers.Open(cfg, o.recipe, recipe, o.name, o.noAttach)
	}

	if o.branch == "" && wantsBranch(cfg) {
		o.branch, err = popup.FzfPickOrType(branches(recipe.StartDirectory), "branch> ", color)
		if err != nil || o.branch == "" {
			return err
		}
	}

	v := ringers.Vars(o.recipe, recipe, o.name, o.branch)
	if err := ringers.Run(cfg.Ringers.Command(cfg.Ringers.Create, v), recipe.StartDirectory); err != nil {
		return fmt.Errorf("create failed (%w); no ringer made", err)
	}
	if dir := cfg.Ringers.DirFor(v); !isDir(dir) {
		return fmt.Errorf("create succeeded but left no %s behind", dir)
	}
	return ringers.Open(cfg, o.recipe, recipe, o.name, o.noAttach)
}

// existingMark tags the names in the picker that already have a ringer.
const existingMark = " (open)"

// choices lists what the name picker offers. With fixed names: the existing
// ringers first (marked), then the free names in their configured order;
// otherwise just the existing ringers (a typed name makes a new one).
func choices(cfg config.Config, existing []string) []string {
	if len(cfg.Ringers.Names) == 0 {
		return existing
	}
	var items []string
	for _, n := range existing {
		items = append(items, n+existingMark)
	}
	return append(items, ringers.Free(cfg, existing)...)
}

// wantsBranch reports whether create uses {branch}, so it's worth asking for.
func wantsBranch(cfg config.Config) bool {
	return strings.Contains(cfg.Ringers.Create, "{branch}")
}

// branches lists the local and remote branches of the repo at dir, newest
// first, without duplicates; nothing when dir isn't in a git repo.
func branches(dir string) []string {
	var names []string
	for _, ref := range []string{"refs/heads", "refs/remotes"} {
		strip := "2"
		if ref == "refs/remotes" {
			strip = "3" // refs/remotes/<remote>/<branch>
		}
		out, err := exec.Command("git", "-C", dir, "for-each-ref", "--sort=-committerdate",
			"--format=%(refname:lstrip="+strip+")", ref).Output()
		if err != nil {
			return nil
		}
		for _, n := range strings.Fields(string(out)) {
			if n != "HEAD" && !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	}
	return names
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
