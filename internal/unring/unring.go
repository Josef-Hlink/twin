package unring

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/ringers"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin unring [<recipe>@]<name> [--force]

drop a ringer: runs [ringers] remove, then kills its session (landing you on
the recipe's own session if you were in it). a failing remove aborts with
its output, so the command decides what is safe to delete.
  <recipe>      defaults to the current session's recipe
  <name>        picked in fzf when omitted
  --force       run [ringers] force-remove instead, for when you want the
                ringer gone along with whatever remove would refuse to delete
`

// options holds the parsed command line.
type options struct {
	recipe string
	name   string // empty: pick it
	force  bool
}

// Run drops a ringer. Inside tmux it runs in a popup, so remove's output (or
// its refusal) stays readable.
func Run(args []string) error {
	o, err := parse(args)
	if err != nil {
		return err
	}
	if tmux.InTmux() {
		return launchPopup(o)
	}
	return unring(o)
}

// RunPicker runs inside the popup spawned by Run, keeping any error on screen.
func RunPicker(args []string) error {
	o, err := parse(args)
	if err == nil {
		err = unring(o)
	}
	return popup.Hold("unring", err)
}

func parse(args []string) (options, error) {
	var o options
	var pos []string
	for _, a := range args {
		switch {
		case a == "--force":
			o.force = true
		case strings.HasPrefix(a, "-"):
			return o, fmt.Errorf("unknown flag %s", a)
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) > 1 {
		return o, errors.New("expected [<recipe>@]<name>")
	}

	target := ""
	if len(pos) == 1 {
		target = pos[0]
	}
	var err error
	o.recipe, o.name, err = ringers.ParseTarget(target)
	return o, err
}

// removeCommand returns the template unring runs (force-remove with --force,
// remove otherwise) and its key, for messages. An unset one is an error.
func removeCommand(r config.Ringers, force bool) (tmpl, key string, err error) {
	tmpl, key = r.Remove, "remove"
	if force {
		tmpl, key = r.ForceRemove, "force-remove"
	}
	if tmpl == "" {
		return "", key, fmt.Errorf("[ringers] has no %s command", key)
	}
	return tmpl, key, nil
}

// launchPopup opens a popup running unring-picker, sized for the recipe's
// ringers and for remove's output.
func launchPopup(o options) error {
	cfg, r, err := ringers.Load(o.recipe)
	if err != nil {
		return err
	}
	existing := ringers.List(cfg, o.recipe, r)
	if len(existing) == 0 {
		fmt.Printf("%s has no ringers\n", o.recipe)
		return nil
	}

	maxLine := 0
	for _, n := range existing {
		maxLine = max(maxLine, len(n))
	}
	width, height := popup.Dims(len(existing), maxLine, cfg.OptionCap(), 0, 0, false)
	width, height = max(width, 72), max(height, 12) // room for remove's output

	cmd := "unring-picker " + popup.Quote(ringers.SessionName(o.recipe, o.name))
	if o.force {
		cmd += " --force"
	}
	return popup.Launch("unring", width, height, cmd, cfg.BorderColor("unring"))
}

// unring runs remove (or force-remove) and, once that succeeded, kills the
// ringer's session.
func unring(o options) error {
	recipeName, name := o.recipe, o.name
	cfg, recipe, err := ringers.Load(recipeName)
	if err != nil {
		return err
	}
	remove, key, err := removeCommand(cfg.Ringers, o.force)
	if err != nil {
		return err
	}
	existing := ringers.List(cfg, recipeName, recipe)

	if name == "" {
		if len(existing) == 0 {
			fmt.Printf("%s has no ringers\n", recipeName)
			return nil
		}
		name, err = popup.FzfSelect(existing, 0, "", cfg.BorderColor("unring"))
		if err != nil || name == "" {
			return err // picker error, or cancelled
		}
	}
	if !slices.Contains(existing, name) {
		return fmt.Errorf("%s has no ringer %q", recipeName, name)
	}
	session := ringers.SessionName(recipeName, name)

	// remove goes before the session: when unring runs inside the ringer's own
	// session, killing it ends unring too, so that has to be the last step.
	v := ringers.Vars(recipeName, recipe, name, "")
	if err := ringers.Run(cfg.Ringers.Command(remove, v), recipe.StartDirectory); err != nil {
		return fmt.Errorf("%s failed (%w); %s kept", key, err, session)
	}

	summary := "dropped " + session
	fmt.Println(summary)
	return closeSession(session, recipeName, summary)
}

// closeSession kills the ringer's session, if it's running. When you're in
// it, you land on the recipe's own session first (or any other one), and the
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
		tmux.DisplayMessage(summary) // the popup closes before you can read it
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
