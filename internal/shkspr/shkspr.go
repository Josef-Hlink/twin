package shkspr

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/Josef-Hlink/twin/internal/config"
)

const Usage = `usage: twin shkspr [--recipes | -r]

open twin.toml in $EDITOR (or vim/nano if unset).
  --recipes, -r open the recipes directory instead of twin.toml
`

// Run opens the twin.toml config file or the recipes directory in the
// user's editor.
func Run(args []string) error {
	fs := flag.NewFlagSet("shkspr", flag.ContinueOnError)
	recipes := fs.Bool("recipes", false, "open the recipes directory")
	fs.BoolVar(recipes, "r", false, "open the recipes directory (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	editor, err := resolveEditor()
	if err != nil {
		return err
	}

	var path string
	if *recipes {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		path = cfg.RecipeDir
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
