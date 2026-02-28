package fr

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/tspmo"
)

// Run opens a single tmux session from a recipe.
// With no args it launches fzf to pick one; with a name arg it opens directly;
// with --list it prints all available recipe names.
func Run(args []string) error {
	if len(args) > 0 && args[0] == "--list" {
		return list()
	}
	if len(args) > 0 {
		return open(args[0])
	}
	return pick()
}

// list prints all available recipe names.
func list() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	names, err := config.ListRecipes(cfg.RecipeDir)
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Println(name)
	}
	return nil
}

// open creates a tmux session from the named recipe.
func open(name string) error {
	if tmux.HasSession(name) {
		fmt.Printf("skipped (already exists): %s\n", name)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	recipe, err := config.LoadRecipe(cfg.RecipeDir, name)
	if err != nil {
		return err
	}

	if err := tspmo.CreateSession(name, recipe); err != nil {
		return fmt.Errorf("creating session %s: %w", name, err)
	}

	fmt.Printf("created: %s\n", name)
	return nil
}

// pick shows an fzf menu of recipes that don't have open sessions, and opens the selection.
func pick() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	names, err := config.ListRecipes(cfg.RecipeDir)
	if err != nil {
		return err
	}

	// Filter out recipes that already have a running session.
	var available []string
	for _, name := range names {
		if !tmux.HasSession(name) {
			available = append(available, name)
		}
	}

	if len(available) == 0 {
		fmt.Println("no unopened recipes available")
		return nil
	}

	selected, err := fzfSelect(available)
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil // user cancelled
	}

	return open(selected)
}

// fzfSelect pipes the given lines to fzf and returns the selected line.
func fzfSelect(items []string) (string, error) {
	cmd := exec.Command("fzf")
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// fzf exits 130 on Escape, 1 on no match — not errors.
			if exitErr.ExitCode() == 130 || exitErr.ExitCode() == 1 {
				return "", nil
			}
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
