# twin — tmux window/workspace manager

## What is twin?

A lightweight CLI tool written in Go for managing tmux sessions from TOML recipes.
Born out of the frustration of manually setting up tmux workspaces every morning.

twin has subcommands:

- `twin tspmo` — spin up tmux sessions from TOML recipes (the main "opener")
- `twin fr` — open a single recipe session (fzf picker, by name, or list)
- `twin sybau` — fzf-based session switcher in a tmux popup (bind to a tmux key)
- `twin ring` — open a ringer: a named copy of a recipe's session, made real by user-configured commands
- `twin unring` — drop a ringer: run its remove (or force-remove) command, kill its session

The name "twin" is short, clean, and works as an acronym (tmux workspace/window something).
Subcommand names are playful slang-inspired:
- "tspmo" = this shit pisses me off
- "fr" = for real
- "sybau" = shut your bitch ass up
- "ring" / "unring" are the exception: plain words. A ringer is a dead ringer of a
  session, and the verbs just say what they do

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
    ring/
      ring.go           # open (and create) a ringer
    unring/
      unring.go         # drop a ringer (remove command, then its session)
    ringers/
      ringers.go        # ringer naming, lookup, opening a ringer's session
    popup/
      popup.go          # tmux popup sizing/launching + fzf pickers
    config/
      config.go         # twin.toml + recipe loading
      ringers.go        # the [ringers] block: templates, placeholders, skip-in-ringers
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

# optional: cap how many rows the popup pickers (fr/sybau/pbqt) are sized for.
# fzf scrolls through any options beyond the cap. omitted/0 = grow to fit all.
max-options = 10

# optional: auto-attach to this session after tspmo finishes (must be in active list).
# if omitted, tspmo prompts the user.
auto-attach-to = "front"

# optional: ringers (see `twin ring`). twin only does the session side; these
# commands make a copy real, however this machine does that.
[ringers]
names  = ["red", "green", "blue"]     # what ring offers; omit to allow any name
dir    = "~/copies/{name}/{recipe}"    # where a ringer's session starts; {name} exactly once
create = "mkcopy {name} {recipe} {branch}"
remove = "rmcopy {name} {recipe}"
force-remove = "rmcopy {name} {recipe} --force" # optional: what unring --force runs
enter  = "copy-shell {name}"           # optional: tmux default-command of a ringer's session
main   = "copy-shell default {recipe}" # optional: the same for a recipe's own session

[ringers.colors]                        # optional: tint per ringer name
red = "red"
```

Placeholders: `{recipe}`, `{name}`, `{branch}`, `{repo}` (the recipe's start-directory),
`{dir}` (the expanded dir). In commands, values are shell-quoted only when needed and an
empty value disappears (so `create … {branch}` without a branch just ends early). In `main`
only `{recipe}` and `{repo}` are set.

**Recipe files** (e.g. `~/.config/twin/recipes/front.toml`):
```toml
start-directory = "~/Developer/pro/my-frontend/"

[[windows]]
name = "editor"             # optional fixed window name
start-directory = "src/"    # relative to recipe start-directory
command = "nvim"

[[windows]]
# empty window, just a shell

[[windows]]
command = "lazygit"

[[windows]]
commands = ["npm install", "make run"]  # list = each entry on its own shell line
```

- Session name = recipe filename (front.toml -> session "front")
- `start-directory` at the top level is required
- `windows` is an ordered array; `name` is optional — without it tmux shows the active command (an explicit name disables tmux's automatic-rename for that window)
- `start-directory` on a window is relative to the recipe's top-level start-directory
- `command = "x"` runs a single command; if omitted the window just opens a shell
- `commands = ["x", "y"]` runs each entry as its own shell line (a short setup sequence). A window uses `command` **or** `commands`, never both
- `~` and environment variables are expanded in paths

**Panes** (optional) — a window can be split into multiple panes instead of
holding a single shell:
```toml
[[windows]]
  [[windows.panes]]
  title = "vim"                # optional pane title, shown in the pane border
  command = "nvim"             # pane 1 = the window's base pane

  [[windows.panes]]
  split = "right"              # new pane goes to the right of pane 1
  size  = "30%"
  command = "lazygit"          # pane 2

  [[windows.panes]]
  split = "down"               # splits pane 2 (the previous pane)
  size  = "50%"
  start-directory = "logs/"
  command = "tail -f app.log"
  focus = true                 # focus this pane when the window opens
```
- Pane 1 is the window's base pane; later panes split an earlier one
- `split-from = <N>` (1-based pane number) chooses which pane to split; omitted defaults to the **previous** pane
- `split` is `"right"` (side-by-side) or `"down"` (stacked); defaults to `"right"`
- `size` is a percentage like `"30%"`, optional (tmux splits evenly when omitted). **% is relative to the pane being split**, not the whole window — needs tmux ≥3.1
- `command`/`commands` and `start-directory` work per-pane (start-directory relative to the window's)
- `title = "x"` titles a pane (`select-pane -T`); when any pane in a window has one, twin sets window-local `pane-border-status top` so titles are visible. Programs emitting terminal-title escapes can overwrite a title later — twin sets it once at creation
- `focus = true` focuses that pane on open (default is pane 1); only one pane per window may set it
- A window uses **either** window-level `command`/`commands` (single pane, the simple form above) **or** `panes`, never both
- `skip-in-ringers = true` on a window or a pane drops its command(s) in ringer sessions — for setups where copies can't run the same dev servers side by side

### Subcommand behavior

**`twin tspmo`** (no additional args):
1. Load twin.toml config
2. Read the `active` list
3. For each active recipe: parse the TOML, create a tmux session with windows/panes/commands
4. Skip recipes whose session name already exists (idempotent)
5. Show real-time spinner progress (braille animation in TTY, plain lines otherwise)
6. When `ordered-sessions` is true (default), add 1s delay between spawns to preserve order
7. Auto-attach to `auto-attach-to` session, or prompt user with `[Y/n]`
8. Attachment uses `switch-client` inside tmux, `attach-session` outside

**`twin fr`**:
- `twin fr` — fzf picker showing recipes that don't have an open session yet
- `twin fr <name>` — open a specific recipe directly (skip if session exists)
- `twin fr --list` — print all available recipe names
- the picker also lists dormant ringers (dir exists, no session), and `twin fr <recipe>@<name>` reopens one
- recipe sessions (fr, tspmo) start every pane through `[ringers] main` when it's set

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

**`twin shkspr`** — open config in `$EDITOR` (falls back to vim, then nano):
- `twin shkspr` — open `twin.toml`
- `twin shkspr <recipe>` — open `<recipe-dir>/<recipe>.toml` (a trailing `.toml` is trimmed, so `dots` and `dots.toml` are equivalent)
- `twin shkspr .` — open the recipe named after the currently attached tmux session (errors outside tmux); in a ringer (`pylona@blue`) that's its recipe
- Opening a recipe name that doesn't exist yet drops you into the editor on a fresh file — a quick way to start a new recipe

**`twin ring [<recipe>@]<name> [branch] [--no-attach]`** — open a ringer, session `<recipe>@<name>`:
- `<recipe>` defaults to the current session's recipe (`blue` inside `pylona@red` means `pylona@blue`)
- `<name>`: no `/ . : @`, quotes, `\`, whitespace, or leading `-`; with `[ringers] names` set, only those
- a ringer exists when its `dir` does; an existing one just gets its session (re)built from the recipe and switched to
- a new one runs `create` (in `{repo}`, terminal attached, Ctrl-C stops the command not twin), then must have left `dir` behind
- name and branch are picked when omitted: fzf of existing + free names (typing a new one when no `names` are set), and of `{repo}`'s branches only when `create` uses `{branch}`. Inside tmux the whole thing runs in a popup (`ring-picker`) so create's output shows
- the session is the recipe rooted at `dir`, `skip-in-ringers` commands dropped, `enter` as its `default-command` (new-session gets it as the first pane's command, set-option for everything after, so manual splits land in the ringer too), tinted from `[ringers.colors]` (status-style, status-left-style and both pane border styles get fg=<color> appended, pane-border-lines heavy; per window plus an after-new-window session hook), and the session user options `@ringer` / `@ringer-color` set for tmux.conf formats

**`twin unring [<recipe>@]<name> [--force]`** — drop a ringer:
- runs `remove` first; a non-zero exit aborts and keeps everything (remove owns any safety checks)
- `--force` runs `force-remove` instead (an error when it's unset; never falls back to `remove`), for throwing away work `remove` would refuse to delete. No confirmation prompt; the popup path passes the flag on to `unring-picker`
- kills the session last; dropping the ringer you're in lands you on the recipe's session first, with a summary on the status line
- tspmo ignores ringers; reopen them with `ring` or `fr`

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

The Go toolchain comes from a per-project nix devShell (`flake.nix`), not a global
install. direnv auto-loads it on `cd` into the repo; if direnv is off, run `nix develop`
to enter the shell manually.

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
