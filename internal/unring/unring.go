package unring

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/ringers"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin unring [<recipe>@]<name>

drop a ringer: runs [ringers] remove, then kills its session (landing you on
the recipe's own session if you were in it). a failing remove aborts with
its output, so the command decides what is safe to delete.
  <recipe>      defaults to the current session's recipe
  <name>        picked in fzf when omitted
`

// Run drops a ringer. Inside tmux it runs in a popup, so remove's output (or
// its refusal) stays readable.
func Run(args []string) error {
	recipe, name, err := parse(args)
	if err != nil {
		return err
	}
	if tmux.InTmux() {
		return launchPopup(recipe, name)
	}
	return unring(recipe, name)
}

// RunPicker runs inside the popup spawned by Run, keeping any error on screen.
func RunPicker(args []string) error {
	recipe, name, err := parse(args)
	if err == nil {
		err = unring(recipe, name)
	}
	return popup.Hold("unring", err)
}

func parse(args []string) (recipe, name string, err error) {
	if len(args) > 1 {
		return "", "", errors.New("expected [<recipe>@]<name>")
	}
	target := ""
	if len(args) == 1 {
		target = args[0]
	}
	return ringers.ParseTarget(target)
}

// launchPopup opens a popup running unring-picker, sized for the recipe's
// ringers and for remove's output.
func launchPopup(recipe, name string) error {
	cfg, r, err := ringers.Load(recipe)
	if err != nil {
		return err
	}
	existing := ringers.List(cfg, recipe, r)
	if len(existing) == 0 {
		fmt.Printf("%s has no ringers\n", recipe)
		return nil
	}

	maxLine := 0
	for _, n := range existing {
		maxLine = max(maxLine, len(n))
	}
	width, height := popup.Dims(len(existing), maxLine, cfg.OptionCap(), 0, 0, false)
	width, height = max(width, 72), max(height, 12) // room for remove's output

	cmd := "unring-picker " + popup.Quote(ringers.SessionName(recipe, name))
	return popup.Launch("unring", width, height, cmd, cfg.BorderColor("unring"))
}

// unring runs remove and, once that succeeded, kills the ringer's session.
func unring(recipeName, name string) error {
	cfg, recipe, err := ringers.Load(recipeName)
	if err != nil {
		return err
	}
	if cfg.Ringers.Remove == "" {
		return errors.New("[ringers] has no remove command")
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
	if err := ringers.Run(cfg.Ringers.Command(cfg.Ringers.Remove, v), recipe.StartDirectory); err != nil {
		return fmt.Errorf("remove failed (%w); %s kept", err, session)
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
