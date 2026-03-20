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
	RecipeDir       string   `toml:"recipe-dir"`
	Active          []string `toml:"active"`
	OrderedSessions *bool    `toml:"ordered-sessions"`
	AutoAttachTo    string   `toml:"auto-attach-to"`
	TysmMsg         string   `toml:"tysm-msg"`
}

// IsOrderedSessions returns whether sessions should be created with delays
// to preserve ordering. Defaults to true when not explicitly set.
func (c Config) IsOrderedSessions() bool {
	if c.OrderedSessions == nil {
		return true
	}
	return *c.OrderedSessions
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
// If no config exists, it scaffolds a starter config and proceeds normally.
// Config dir defaults to ~/.config/twin, overridable via TWIN_CONFIG_DIR
// or XDG_CONFIG_HOME.
func Load() (Config, error) {
	dir := configDir()
	path := filepath.Join(dir, "twin.toml")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := scaffold(dir); err != nil {
			return Config{}, fmt.Errorf("scaffolding config: %w", err)
		}
		fmt.Printf("no config found — created starter config at %s\n", configDirVar())
		fmt.Println("run `twin fr twin` to open the twin recipe")
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("loading %s: %w", path, err)
	}

	cfg.RecipeDir = expandPath(cfg.RecipeDir)
	return cfg, nil
}

// LoadRecipe reads a recipe TOML file from the recipe directory.
func LoadRecipe(recipeDir, name string) (Recipe, error) {
	path := filepath.Join(recipeDir, name+".toml")

	var r Recipe
	if _, err := toml.DecodeFile(path, &r); err != nil {
		return r, fmt.Errorf("loading recipe %s: %w", path, err)
	}

	r.StartDirectory = expandPath(r.StartDirectory)
	return r, nil
}

// ListRecipes returns the names of all recipe files (without .toml extension)
// found in the given directory.
func ListRecipes(recipeDir string) ([]string, error) {
	entries, err := os.ReadDir(recipeDir)
	if err != nil {
		return nil, fmt.Errorf("reading recipe dir %s: %w", recipeDir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".toml"))
	}
	return names, nil
}

func configDir() string {
	if dir := os.Getenv("TWIN_CONFIG_DIR"); dir != "" {
		return expandPath(dir)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(expandPath(xdg), "twin")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "twin")
}

// scaffold creates the config directory structure and writes starter files:
// twin.toml, recipes/home.toml, and recipes/twin.toml.
func scaffold(dir string) error {
	recipeDir := filepath.Join(dir, "recipes")
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		return fmt.Errorf("creating config dirs: %w", err)
	}

	cfgContent := fmt.Sprintf("recipe-dir = %q\nactive = [\"home\"]\n", recipeDirVar())

	homeRecipe := `start-directory = "~/"

[[windows]]
commands = ["ls -lAh"]

[[windows]]
start-directory = ".config/"
commands = ["if [ -d nvim ]; then vim nvim/; elif [ -f ~/.vimrc ]; then vim ~/.vimrc; fi"]

[[windows]]
commands = ["cd ` + configDirVar() + `", "echo \"hi twin, here are your recipes:\"", "cat recipes/*.toml"]
`

	twinRecipe := fmt.Sprintf(`start-directory = %q

[[windows]]

[[windows]]
commands = ["cd ${GOPATH:-$HOME/go}/bin && ls -la twin"]
`, configDirVar())

	files := map[string]string{
		filepath.Join(dir, "twin.toml"):       cfgContent,
		filepath.Join(recipeDir, "home.toml"): homeRecipe,
		filepath.Join(recipeDir, "twin.toml"): twinRecipe,
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return nil
}

// configDirVar returns an unexpanded path string suitable for writing into
// scaffolded TOML files. It uses whichever env var the user has set.
func configDirVar() string {
	if os.Getenv("TWIN_CONFIG_DIR") != "" {
		return "$TWIN_CONFIG_DIR"
	}
	if os.Getenv("XDG_CONFIG_HOME") != "" {
		return "$XDG_CONFIG_HOME/twin"
	}
	return "~/.config/twin"
}

// recipeDirVar returns configDirVar() + "/recipes".
func recipeDirVar() string {
	return configDirVar() + "/recipes"
}

// expandPath expands environment variables and a leading ~ in the path.
func expandPath(path string) string {
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
