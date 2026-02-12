# twin — tmux window/workspace manager

## What is twin?

A lightweight CLI tool written in Go for managing tmux sessions from TOML recipes.
Born out of the frustration of manually setting up tmux workspaces every morning.

twin has subcommands:

- `twin tspmo` — spin up tmux sessions from TOML recipes (the main "opener")
- `twin sybau` — fzf-based session switcher (bind to a tmux key for quick switching)

The name "twin" is short, clean, and works as an acronym (tmux workspace/window something).
"tspmo" and "sybau" are playful slang-inspired names (ts pmo = this shit pisses me off,
sybau = shut your bitch ass up).

## Architecture

### Project structure

```
twin/
  cmd/
    twin/
      main.go           # CLI entrypoint, subcommand dispatch
  internal/
    tspmo/
      tspmo.go          # recipe parsing + tmux session creation
    sybau/
      sybau.go          # fzf session picker + switcher
    config/
      config.go         # twin.toml config loading
    tmux/
      tmux.go           # thin wrapper around tmux CLI commands
  go.mod
  go.sum
  CLAUDE.md
  README.md
```

### Config & recipes

Config lives at `~/.config/twin/twin.toml` (XDG). Path overridable via `TWIN_CONFIG_DIR` env var.

**twin.toml** — main config:
```toml
recipe-dir = "~/.config/twin/recipes"  # where to find recipe files

# which recipes to spin up (filenames without .toml extension)
active = ["front", "back", "infra"]
```

**Recipe files** (e.g. `~/.config/twin/recipes/front.toml`):
```toml
start-directory = "~/Developer/pro/my-frontend/"

[[windows]]
start-directory = "src/"    # relative to recipe start-directory
commands = ["nvim"]

[[windows]]
# empty window, just a shell

[[windows]]
commands = ["lazygit"]

[[windows]]
commands = ["make run"]
```

- Session name = recipe filename (front.toml -> session "front")
- `start-directory` at the top level is required
- `windows` is an ordered array; window names are optional (tmux shows the active command by default)
- `start-directory` on a window is relative to the recipe's top-level start-directory
- `commands` is optional; if omitted the window just opens a shell
- No pane/split support — one pane per window, keep it simple

### Subcommand behavior

**`twin tspmo`** (no additional args):
1. Load twin.toml config
2. Read the `active` list
3. For each active recipe: parse the TOML, create a tmux session with windows/commands
4. Skip recipes whose session name already exists (idempotent)
5. Print a summary of what was created/skipped

**`twin sybau`**:
1. Get list of running tmux sessions
2. Pipe to fzf for fuzzy selection
3. Switch to the selected session
4. Designed to be bound in tmux.conf: `bind-key Space run-shell "twin sybau"`

**Teardown**: not a twin concern — just use `tmux kill-server` or kill individual sessions manually.

## Development guidelines

- **Keep it lightweight.** No frameworks, no unnecessary abstractions. Standard library + minimal deps.
- **Dependencies**: only what's truly needed. Currently: `BurntSushi/toml` for TOML parsing. fzf is called as an external command (not embedded).
- **Go style**: follow standard Go conventions (gofmt, effective Go idioms). This is a learning project — prefer clarity over cleverness.
- **Error handling**: simple and direct. Log errors with context, exit non-zero on failure. No complex error wrapping chains.
- **No over-engineering**: no plugin systems, no dynamic loading, no feature flags. If a feature isn't in this doc, don't add it.

## Go learning context

The author is a Go beginner. When implementing:
- Explain Go patterns and idioms as they come up (briefly, not lectures)
- Point out when something is "the Go way" vs alternatives
- Build incrementally — get something working, then refine
