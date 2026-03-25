# twin — tmux window/workspace manager

## What is twin?

A lightweight CLI tool written in Go for managing tmux sessions from TOML recipes.
Born out of the frustration of manually setting up tmux workspaces every morning.

twin has subcommands:

- `twin tspmo` — spin up tmux sessions from TOML recipes (the main "opener")
- `twin fr` — open a single recipe session (fzf picker, by name, or list)
- `twin sybau` — fzf-based session switcher in a tmux popup (bind to a tmux key)

The name "twin" is short, clean, and works as an acronym (tmux workspace/window something).
Subcommand names are playful slang-inspired:
- "tspmo" = this shit pisses me off
- "fr" = for real
- "sybau" = shut your bitch ass up

## Architecture

### Project structure

```
twin/
  cmd/
    twin/
      main.go           # CLI entrypoint, subcommand dispatch
  internal/
    tspmo/
      tspmo.go          # bulk session creation from active recipes
      spinner.go         # braille unicode progress spinner
    fr/
      fr.go             # single recipe opener (fzf picker / name / --list)
    sybau/
      sybau.go          # fzf session picker in tmux popup
    config/
      config.go         # twin.toml + recipe loading
    tmux/
      tmux.go           # thin wrapper around tmux CLI commands
  .github/
    workflows/
      lint-pr.yml       # conventional commit PR title enforcement
  go.mod
  go.sum
  CLAUDE.md
```

### Config & recipes

Config lives at `~/.config/twin/twin.toml` (XDG). Path overridable via `TWIN_CONFIG_DIR` env var.

**twin.toml** — main config:
```toml
recipe-dir = "~/.config/twin/recipes"  # where to find recipe files

# which recipes to spin up with tspmo (filenames without .toml extension)
active = ["front", "back", "infra"]

# optional: preserve session creation order by adding a 1s delay between spawns
# also enables session numbering in sybau picker. defaults to true.
ordered-sessions = true

# optional: auto-attach to this session after tspmo finishes (must be in active list).
# if omitted, tspmo prompts the user.
auto-attach-to = "front"
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
- `~` and environment variables are expanded in paths
- No pane/split support — one pane per window, keep it simple

### Subcommand behavior

**`twin tspmo`** (no additional args):
1. Load twin.toml config
2. Read the `active` list
3. For each active recipe: parse the TOML, create a tmux session with windows/commands
4. Skip recipes whose session name already exists (idempotent)
5. Show real-time spinner progress (braille animation in TTY, plain lines otherwise)
6. When `ordered-sessions` is true (default), add 1s delay between spawns to preserve order
7. Auto-attach to `auto-attach-to` session, or prompt user with `[Y/n]`
8. Attachment uses `switch-client` inside tmux, `attach-session` outside

**`twin fr`**:
- `twin fr` — fzf picker showing recipes that don't have an open session yet
- `twin fr <name>` — open a specific recipe directly (skip if session exists)
- `twin fr --list` — print all available recipe names

**`twin sybau`**:
1. Spawn a tmux popup containing an fzf session picker
2. Excludes the currently attached session from the list
3. Switch to the selected session
4. `--preview` flag shows each session's windows in a right-side preview pane
5. When `ordered-sessions` is true, sessions show numbered as `[N] name`
6. Designed to be bound in tmux.conf: `bind-key Space run-shell "twin sybau --preview"`

**`sybau-picker`** — internal subcommand that runs inside the tmux popup spawned by `sybau`.
Not meant to be called directly. Uses `os.Executable()` for the absolute path since the
popup shell doesn't inherit PATH context.

**Teardown**: not a twin concern — just use `tmux kill-server` or kill individual sessions manually.

## Development workflow

### Git & GitHub

- **Issues first**: features and bugs get a GitHub issue before work starts
- **Branch naming**: `<issue-number>/<slug>` for issue work, `chore/<slug>` for non-issue work
  - Examples: `23/fr-popup-attach`, `chore/extract-usage-text`
- **Commits**: follow 50/72 rule (50 char subject, 72 char body wrap). No conventional commit format on branch commits.
- **PR titles**: conventional commit format enforced by CI (`feat(scope):`, `fix(scope):`, etc.)
- **Squash merge**: `gh pr merge <n> --squash --delete-branch` — GitHub auto-appends `(#n)` to the subject, body left empty
- **CI**: `lint-pr.yml` runs `amannn/action-semantic-pull-request@v6` on PR titles

### Building

```sh
go build ./cmd/twin    # produces ./twin binary
```

`.gitignore` uses `/twin` (leading slash) to ignore the binary without matching `cmd/twin/`.

### Code guidelines

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
