package fr

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/tmux"
	"github.com/Josef-Hlink/twin/internal/tspmo"
)

// Run opens a single tmux session from a recipe.
// With no args it launches a popup picker (or inline fzf outside tmux);
// with a name arg it opens directly and auto-attaches;
// with --list it prints all available recipe names.
func Run(args []string) error {
	noAttach := false
	var filtered []string
	for _, a := range args {
		if a == "--no-attach" {
			noAttach = true
		} else {
			filtered = append(filtered, a)
		}
	}

	if len(filtered) > 0 && filtered[0] == "--list" {
		return list()
	}
	if len(filtered) > 0 {
		return open(filtered[0], noAttach)
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

// open creates a tmux session from the named recipe and auto-attaches.
// If the session already exists, it prompts to attach instead of skipping.
func open(name string, noAttach bool) error {
	if tmux.HasSession(name) {
		if noAttach {
			fmt.Printf("session %q already exists\n", name)
			return nil
		}
		fmt.Printf("session %q already exists, attach? [Y/n] ", name)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "" && answer != "y" && answer != "yes" {
			return nil
		}
		return goToSession(name)
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

	if noAttach {
		return nil
	}
	return goToSession(name)
}

// pick shows a picker of recipes that don't have open sessions.
// Inside tmux it uses a popup; outside tmux it falls back to inline fzf.
func pick() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	names, err := config.ListRecipes(cfg.RecipeDir)
	if err != nil {
		return err
	}

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

	if tmux.InTmux() {
		return pickPopup(available)
	}
	return pickInline(available)
}

// pickPopup launches a tmux popup with the fr-picker subcommand.
func pickPopup(available []string) error {
	maxLine := len("fr") // minimum width
	for _, name := range available {
		if len(name) > maxLine {
			maxLine = len(name)
		}
	}

	width, height := popup.Dims(len(available), maxLine, 0, 0, false)
	return popup.Launch("fr", width, height, "fr-picker")
}

// pickInline shows an inline fzf picker (fallback for outside tmux).
func pickInline(available []string) error {
	selected, err := popup.FzfSelect(available, 0, "")
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil
	}
	return open(selected, false)
}

// RunPicker runs inside the tmux popup spawned by pickPopup.
// It lists unopened recipes, lets the user pick via fzf, creates the session,
// and switches to it.
func RunPicker() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	names, err := config.ListRecipes(cfg.RecipeDir)
	if err != nil {
		return err
	}

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

	selected, err := popup.FzfSelect(available, 0, "")
	if err != nil {
		return fmt.Errorf("fzf: %w", err)
	}
	if selected == "" {
		return nil
	}

	recipe, err := config.LoadRecipe(cfg.RecipeDir, selected)
	if err != nil {
		return err
	}

	if err := tspmo.CreateSession(selected, recipe); err != nil {
		return fmt.Errorf("creating session %s: %w", selected, err)
	}

	return tmux.SwitchClient(selected)
}

// goToSession switches to or attaches to the named session, depending on
// whether we're already inside tmux.
func goToSession(name string) error {
	if tmux.InTmux() {
		return tmux.SwitchClient(name)
	}
	return tmux.AttachSession(name)
}
