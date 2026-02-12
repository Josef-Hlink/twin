package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config represents the top-level twin.toml file.
type Config struct {
	RecipeDir string   `toml:"recipe-dir"`
	Active    []string `toml:"active"`
}

// Window represents a single window in a recipe.
type Window struct {
	StartDirectory string   `toml:"start-directory"`
	Commands       []string `toml:"commands"`
}

// Recipe represents a single recipe TOML file.
type Recipe struct {
	StartDirectory string   `toml:"start-directory"`
	Windows        []Window `toml:"windows"`
}

// Load reads twin.toml from the config directory.
// Config dir defaults to ~/.config/twin, overridable via TWIN_CONFIG_DIR.
func Load() (Config, error) {
	dir := configDir()
	path := filepath.Join(dir, "twin.toml")

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("loading %s: %w", path, err)
	}

	cfg.RecipeDir = expandTilde(cfg.RecipeDir)
	return cfg, nil
}

// LoadRecipe reads a recipe TOML file from the recipe directory.
func LoadRecipe(recipeDir, name string) (Recipe, error) {
	path := filepath.Join(recipeDir, name+".toml")

	var r Recipe
	if _, err := toml.DecodeFile(path, &r); err != nil {
		return r, fmt.Errorf("loading recipe %s: %w", path, err)
	}

	r.StartDirectory = expandTilde(r.StartDirectory)
	return r, nil
}

func configDir() string {
	if dir := os.Getenv("TWIN_CONFIG_DIR"); dir != "" {
		return expandTilde(dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "twin")
}

// expandTilde replaces a leading ~ with the user's home directory.
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
