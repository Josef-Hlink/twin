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
			fmt.Fprintf(os.Stderr, "skipping %s: %v\n", name, err)
			continue
		}

		// Sleep before creating the next session so tmux assigns distinct
		// creation timestamps, preserving the order from the active list.
		if ordered && created > 0 {
			time.Sleep(1 * time.Second)
		}

		p.start(name)

		if err := CreateSession(name, recipe); err != nil {
			fmt.Fprintf(os.Stderr, "error creating %s: %v\n", name, err)
			continue
		}

		p.markDone(name)
		created++
	}

	p.halt()

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

// CreateSession builds a tmux session from a recipe: creates windows and sends commands.
func CreateSession(name string, recipe config.Recipe) error {
	// The first window is created with the session itself (window index 1,
	// since tmux base-index is commonly 1, but we use the default here).
	baseDir := recipe.StartDirectory

	firstWindowDir := baseDir
	if len(recipe.Windows) > 0 && recipe.Windows[0].StartDirectory != "" {
		firstWindowDir = filepath.Join(baseDir, recipe.Windows[0].StartDirectory)
	}

	if err := tmux.NewSession(name, firstWindowDir); err != nil {
		return fmt.Errorf("new-session: %w", err)
	}

	// Send commands to the first window.
	if len(recipe.Windows) > 0 {
		for _, cmd := range recipe.Windows[0].Commands {
			if err := tmux.SendKeys(name+":1", cmd); err != nil {
				return fmt.Errorf("send-keys to %s:1: %w", name, err)
			}
		}
	}

	// Create remaining windows (index 2, 3, ...).
	for i := 1; i < len(recipe.Windows); i++ {
		w := recipe.Windows[i]
		winDir := baseDir
		if w.StartDirectory != "" {
			winDir = filepath.Join(baseDir, w.StartDirectory)
		}

		target := fmt.Sprintf("%s:%d", name, i+1)
		if err := tmux.NewWindow(target, winDir); err != nil {
			return fmt.Errorf("new-window %s: %w", target, err)
		}

		for _, cmd := range w.Commands {
			if err := tmux.SendKeys(target, cmd); err != nil {
				return fmt.Errorf("send-keys to %s: %w", target, err)
			}
		}
	}

	// Select the first window so the session starts there.
	tmux.SelectWindow(name + ":1")

	return nil
}
