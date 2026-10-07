# twin — tmux window/workspace manager

## What is twin?

A lightweight CLI tool written in Go for managing tmux sessions from TOML recipes.
Born out of the frustration of manually setting up tmux workspaces every morning.

twin has subcommands:

- `twin tspmo` — spin up tmux sessions from TOML recipes (the main "opener")
- `twin fr` — open a single recipe session (fzf picker, by name, or list)
- `twin sybau` — fzf-based session switcher in a tmux popup (bind to a tmux key)
- `twin wth` — hatch a twin: a git worktree of a recipe's repo with its own session
- `twin wtf` — fold a twin: remove its worktree, session, and (when safe) branch

The name "twin" is short, clean, and works as an acronym (tmux workspace/window something).
Subcommand names are playful slang-inspired:
- "tspmo" = this shit pisses me off
- "fr" = for real
- "sybau" = shut your bitch ass up
- "wth" = what the hell (worktree hatch)
- "wtf" = what the fuck (worktree fold)

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
      spinner.go        # braille unicode progress spinner
    fr/
      fr.go             # single recipe opener (fzf picker / name / --list)
    frfr/
      frfr.go           # scaffold a recipe from template.toml
    sybau/
      sybau.go          # fzf session picker in tmux popup
    pbqt/
      pbqt.go           # kill a session (fzf picker / name)
    icl/
      icl.go            # tabbed quickview of running claude panes
    shkspr/
      shkspr.go         # open twin.toml or a recipe in $EDITOR
    tysm/
      tysm.go           # kill the tmux server with a farewell
    wth/
      wth.go            # hatch a twin (worktree + bootstrap + session)
    wtf/
      wtf.go            # fold a twin (safety checks, worktree, branch, session)
    twins/
      twins.go          # twin naming, repo/twin lookup, opening a twin's session
    config/
      config.go         # twin.toml + recipe loading
      color.go          # popup accent colors for tmux/fzf
    popup/
      popup.go          # tmux popup sizing/launching + fzf pickers
    tmux/
      tmux.go           # thin wrapper around tmux CLI commands
    git/
      git.go            # thin wrapper around git CLI commands
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

# optional: bootstrapping shared by every recipe's twins (see `twin wth`).
# recipes can add their own [worktree] block; the lists are appended.
[worktree]
copy = [".envrc", ".env", "jdh", ".claude/settings.local.json"]
```

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

**Twins** (optional) — how a recipe behaves when `twin wth` opens it on a git worktree:
```toml
[[windows]]
command = "pnpm dev"
skip-in-wt = true              # in twins this window opens as a plain shell

[worktree]                     # appended to twin.toml's [worktree] lists
copy  = ["apps/backend/.env"]  # untracked paths (from the repo root) to copy over
setup = ["pnpm install"]       # run once in a fresh twin, inside its direnv env
```
- `skip-in-wt = true` on a window or a pane drops its command(s) in twin sessions — for dev servers and workers that would clash with the original's
- `copy` entries are paths from the repo root, copied from the main checkout; missing ones are skipped silently, ones git already tracks are left alone
- `setup` runs in the worktree root before the session is built; a failure (or Ctrl-C) rolls the hatch back

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
- the picker also lists dormant twins (worktree exists, no session), and `twin fr <recipe>@<name>` reopens one

**`twin sybau`**:
1. Spawn a tmux popup containing an fzf session picker
2. Excludes the currently attached session from the list
3. Switch to the selected session
4. `--preview` flag shows each session's windows in a right-side preview pane
5. When `ordered-sessions` is true, sessions show numbered as `[N] name`
6. Designed to be bound in tmux.conf: `bind-key Space run-shell "twin sybau --preview"`

Twins are listed right under their recipe's session, indented and unnumbered.

**`sybau-picker`** — internal subcommand that runs inside the tmux popup spawned by `sybau`.
Not meant to be called directly. Uses `os.Executable()` for the absolute path since the
popup shell doesn't inherit PATH context.

**`twin shkspr`** — open config in `$EDITOR` (falls back to vim, then nano):
- `twin shkspr` — open `twin.toml`
- `twin shkspr <recipe>` — open `<recipe-dir>/<recipe>.toml` (a trailing `.toml` is trimmed, so `dots` and `dots.toml` are equivalent)
- `twin shkspr .` — open the recipe named after the currently attached tmux session (errors outside tmux); in a twin (`pylona@blue`) that's its recipe
- Opening a recipe name that doesn't exist yet drops you into the editor on a fresh file — a quick way to start a new recipe

**`twin wth [<recipe>@]<name> [branch] [--no-attach]`** — worktree hatch. A twin
is a git worktree of a recipe's repo with its own session `<recipe>@<name>`:
- `<recipe>` defaults to the current session's recipe (`blue` inside `pylona@red` means `pylona@blue`)
- `<name>` is the twin's id and its folder `<repo>.twins/<name>/`, next to the main checkout; no `/ . : @`, whitespace, or leading `-`
- name and branch are prompted for when omitted, never inferred: fzf of the repo's twins / of free local + remote branches, where typing a new value works too (alt-enter takes the typed text over a fuzzy match). Inside tmux the whole hatch runs in a popup (`wth-picker`)
- an existing twin just gets its session (re)built from the recipe and switched to
- a new twin: `git fetch`, then `git worktree add` on the branch (an existing local or remote one, or a new one from origin's default branch with no upstream), then bootstrap: copy the `[worktree]` files, `direnv allow` if an `.envrc` landed (and load it once, so the panes don't all build the environment at the same moment), run `setup`
- the session is the recipe with its start-directory moved into the worktree (keeping any offset below the repo root) and `skip-in-wt` commands dropped

**`twin wtf [<recipe>@]<name>`** — worktree fold:
- refuses, before touching anything, when the worktree has modified or untracked files (ignored ones, like the copied files, are fine) or a detached HEAD with commits no ref contains
- `git fetch --prune`, `git worktree remove` (never `--force`), removes `<repo>.twins/` once empty
- deletes the branch only when that loses nothing: its upstream is `[gone]` (merged + deleted PR; squash merges defeat `git branch -d`) or it has no commits beyond origin's default branch
- kills the session last; folding the twin you're in lands you on the recipe's session first, with a summary on the status line
- tspmo ignores twins; reopen them with `wth` or `fr`

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
- **Dependencies**: only what's truly needed. Currently: `BurntSushi/toml` for TOML parsing. fzf and git are called as external commands (not embedded).
- **Go style**: follow standard Go conventions (gofmt, effective Go idioms). This is a learning project — prefer clarity over cleverness.
- **Error handling**: simple and direct. Log errors with context, exit non-zero on failure. No complex error wrapping chains.
- **No over-engineering**: no plugin systems, no dynamic loading, no feature flags. If a feature isn't in this doc, don't add it.

## Go learning context

The author is a Go beginner. When implementing:
- Explain Go patterns and idioms as they come up (briefly, not lectures)
- Point out when something is "the Go way" vs alternatives
- Build incrementally — get something working, then refine
