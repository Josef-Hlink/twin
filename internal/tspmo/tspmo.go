package tspmo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

// Run loads the config, reads active recipes, and creates tmux sessions.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var created, skipped []string

	for _, name := range cfg.Active {
		if tmux.HasSession(name) {
			skipped = append(skipped, name)
			continue
		}

		recipe, err := config.LoadRecipe(cfg.RecipeDir, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipping %s: %v\n", name, err)
			continue
		}

		if err := createSession(name, recipe); err != nil {
			fmt.Fprintf(os.Stderr, "error creating %s: %v\n", name, err)
			continue
		}

		created = append(created, name)
	}

	// Print summary.
	if len(created) > 0 {
		fmt.Printf("created: %v\n", created)
	}
	if len(skipped) > 0 {
		fmt.Printf("skipped (already exists): %v\n", skipped)
	}
	if len(created) == 0 && len(skipped) == 0 {
		fmt.Println("no active recipes configured")
	}

	return nil
}

func createSession(name string, recipe config.Recipe) error {
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
