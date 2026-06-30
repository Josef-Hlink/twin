package tspmo

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin tspmo

spin up tmux sessions from active recipes in twin.toml.
sessions whose name already exists are skipped (✓ already exists).
attaches to auto-attach-to from config, or prompts otherwise.
`

// Run loads the config, reads active recipes, and creates tmux sessions.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(cfg.Active) == 0 {
		fmt.Println("no active recipes configured")
		return nil
	}

	tty := isTTY()
	p := newProgress(cfg.Active, tty)
	ordered := cfg.IsOrderedSessions()
	created := 0

	for _, name := range cfg.Active {
		if tmux.HasSession(name) {
			p.skip(name)
			continue
		}

		recipe, err := config.LoadRecipe(cfg.RecipeDir, name)
		if err != nil {
			p.fail(name, err)
			continue
		}

		// Mark active before the inter-session delay so the spinner animates
		// during the wait, not just during the brief CreateSession call.
		p.start(name)

		// Sleep before creating the next session so tmux assigns distinct
		// creation timestamps, preserving the order from the active list.
		if ordered && created > 0 {
			time.Sleep(1 * time.Second)
		}

		if err := CreateSession(name, recipe); err != nil {
			p.fail(name, err)
			continue
		}

		p.markDone(name)
		created++
	}

	p.halt()
	p.reportFailures()

	// Fail loud if any active recipe couldn't start. Done before auto-attach so
	// the failure summary isn't buried behind an attached session.
	if p.failed_ > 0 {
		return fmt.Errorf("%d recipe(s) failed to start", p.failed_)
	}

	// Auto-attach after session creation.
	if p.done_ == 0 {
		return nil
	}
	if !tty {
		return nil
	}

	target, err := attachTarget(cfg)
	if err != nil {
		return err
	}
	if target == "" {
		return nil
	}

	if tmux.InTmux() {
		return tmux.SwitchClient(target)
	}
	return tmux.AttachSession(target)
}

// attachTarget determines which session to attach to.
// Returns empty string if the user declines.
func attachTarget(cfg config.Config) (string, error) {
	if cfg.AutoAttachTo != "" {
		if !slices.Contains(cfg.Active, cfg.AutoAttachTo) {
			return "", fmt.Errorf("auto-attach-to %q is not in active list", cfg.AutoAttachTo)
		}
		return cfg.AutoAttachTo, nil
	}

	// Prompt the user, defaulting to the first active session.
	target := cfg.Active[0]
	fmt.Printf("attach to %s? [Y/n] ", target)

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "" || answer == "y" || answer == "yes" {
		return target, nil
	}
	return "", nil
}

// CreateSession builds a tmux session from a recipe: creates windows, splits
// panes, and sends commands.
func CreateSession(name string, recipe config.Recipe) error {
	baseDir := recipe.StartDirectory

	for i, w := range recipe.Windows {
		// windowDir is where the window's base pane opens; a per-pane
		// start-directory on pane 1 narrows it further (matching window dirs).
		windowDir := baseDir
		if w.StartDirectory != "" {
			windowDir = filepath.Join(baseDir, w.StartDirectory)
		}
		basePaneDir := windowDir
		if len(w.Panes) > 0 && w.Panes[0].StartDirectory != "" {
			basePaneDir = filepath.Join(windowDir, w.Panes[0].StartDirectory)
		}

		// The first window is created with the session itself; the rest are
		// new windows. Both hand back the base pane's stable pane-id.
		var basePaneID string
		var err error
		if i == 0 {
			if basePaneID, err = tmux.NewSession(name, basePaneDir); err != nil {
				return fmt.Errorf("new-session: %w", err)
			}
		} else {
			target := fmt.Sprintf("%s:%d", name, i+1)
			if basePaneID, err = tmux.NewWindow(target, basePaneDir); err != nil {
				return fmt.Errorf("new-window %s: %w", target, err)
			}
		}

		if err := buildWindow(basePaneID, w, windowDir); err != nil {
			return fmt.Errorf("window %d: %w", i+1, err)
		}
	}

	// Select the first window so the session starts there.
	tmux.SelectWindow(name + ":1")

	return nil
}

// buildWindow populates an already-created window. With no panes it sends the
// window-level commands to the base pane (the original single-pane behavior).
// With panes it splits the tree, sends per-pane commands, and focuses one pane.
func buildWindow(basePaneID string, w config.Window, windowDir string) error {
	if len(w.Panes) == 0 {
		return sendCommands(basePaneID, w.Cmds())
	}

	paneIDs := make([]string, len(w.Panes))
	paneIDs[0] = basePaneID
	if err := sendCommands(basePaneID, w.Panes[0].Cmds()); err != nil {
		return err
	}

	for i := 1; i < len(w.Panes); i++ {
		p := w.Panes[i]

		// split-from is 1-based; 0 means the previous pane.
		targetIdx := i - 1
		if p.SplitFrom > 0 {
			targetIdx = p.SplitFrom - 1
		}

		paneDir := windowDir
		if p.StartDirectory != "" {
			paneDir = filepath.Join(windowDir, p.StartDirectory)
		}

		id, err := tmux.SplitPane(paneIDs[targetIdx], paneDir, p.Split, p.Size)
		if err != nil {
			return fmt.Errorf("split pane %d: %w", i+1, err)
		}
		paneIDs[i] = id

		if err := sendCommands(id, p.Cmds()); err != nil {
			return err
		}
	}

	// Focus the requested pane, defaulting to the base pane.
	focus := paneIDs[0]
	for i, p := range w.Panes {
		if p.Focus {
			focus = paneIDs[i]
		}
	}
	tmux.SelectPane(focus)

	return nil
}

// sendCommands dispatches each command to a tmux target (pane-id or window).
func sendCommands(target string, commands []string) error {
	for _, cmd := range commands {
		if err := tmux.SendKeys(target, cmd); err != nil {
			return fmt.Errorf("send-keys to %s: %w", target, err)
		}
	}
	return nil
}
