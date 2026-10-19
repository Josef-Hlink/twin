package shkspr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin shkspr [recipe]

open twin.toml in $EDITOR (or vim/nano if unset).
  recipe  open that recipe file instead (e.g. "twin shkspr dots" -> dots.toml)
          "." means the current tmux session's recipe
`

// Run opens the twin.toml config file, or a single recipe file when a recipe
// name is given, in the user's editor.
func Run(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("expected at most one recipe name, got %d", len(args))
	}

	editor, err := resolveEditor()
	if err != nil {
		return err
	}

	var path string
	if len(args) == 1 {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		name := args[0]
		if name == "." {
			if !tmux.InTmux() {
				return fmt.Errorf(`"." only works inside tmux`)
			}
			name, err = tmux.CurrentSession()
			if err != nil {
				return fmt.Errorf("could not resolve current session: %w", err)
			}
		}
		name = strings.TrimSuffix(name, ".toml")
		path = filepath.Join(cfg.RecipeDir, name+".toml")
	} else {
		path = config.ConfigPath()
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// resolveEditor returns the first available editor from:
// $EDITOR, vim, nano.
func resolveEditor() (string, error) {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor, nil
	}
	for _, name := range []string{"vim", "nano"} {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("no valid editor found on system (set $EDITOR)")
}
