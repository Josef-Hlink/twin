package shkspr

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/Josef-Hlink/twin/internal/config"
)

// Run opens the twin.toml config file in the user's editor.
func Run(args []string) error {
	editor, err := resolveEditor()
	if err != nil {
		return err
	}

	path := config.ConfigPath()

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
