package frfr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
)

const Usage = `usage: twin frfr <name>

scaffold a new recipe from template.toml and open it in $EDITOR.
fails if <name>.toml already exists.
`

// Run creates a new recipe file by copying template.toml, then opens it in
// the user's editor.
func Run(args []string) error {
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, Usage)
		return fmt.Errorf("expected exactly one recipe name")
	}

	name := args[0]
	if name == "template" {
		return fmt.Errorf("%q is a reserved recipe name", name)
	}
	if strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid recipe name %q", name)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	dst := filepath.Join(cfg.RecipeDir, name+".toml")
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("recipe %q already exists at %s", name, dst)
	}

	src := config.TemplatePath(cfg.RecipeDir)
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading template: %w", err)
	}

	if err := os.WriteFile(dst, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}

	editor, err := resolveEditor()
	if err != nil {
		return err
	}

	cmd := exec.Command(editor, dst)
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
