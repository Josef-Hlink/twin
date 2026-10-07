# twin

A lightweight CLI tool for managing tmux sessions from TOML recipes.

Born out of the frustration of manually setting up tmux workspaces every morning.

## Requirements

- Go 1.24+
- [tmux](https://github.com/tmux/tmux)
- [fzf](https://github.com/junegunn/fzf)
- git, for `wth`/`wtf`

## Install

```sh
go install github.com/Josef-Hlink/twin/cmd/twin@latest
```

## Getting started

Run any twin command with no config in place and twin scaffolds a starter setup at `$XDG_CONFIG_HOME/twin/`
(falls back to `~/.config/twin/`).

```
$ twin tspmo
no config found — created starter config at ~/.config/twin
```

This creates:

```
~/.config/twin/
  twin.toml              # main config
  recipes/
    home.toml            # starter recipe (active by default)
    twin.toml            # meta recipe for poking around twin's own config
```

From there, add your own recipes and optionally list them as active in `twin.toml`.

## Config

Config lives at `~/.config/twin/twin.toml`.
Override the location with `TWIN_CONFIG_DIR` or `XDG_CONFIG_HOME`.

```toml
recipe-dir = "~/.config/twin/recipes"
active = ["front", "back", "infra"]
```

`active` controls which recipes `twin tspmo` spins up.

### Popup colors

Each subcommand that uses a popup picker (`fr`, `sybau`, `pbqt`, `icl`, `wth`, `wtf`) has a configurable accent color that paints both the tmux popup border and fzf's pointer/marker, so the picker reads as one themed surface.

```toml
fr-border-color    = "green"      # default: green
sybau-border-color = "magenta"    # default: magenta
pbqt-border-color  = "red"        # default: red
icl-border-color   = "colour214"  # default: colour214 (orange)
wth-border-color   = "blue"       # default: blue
wtf-border-color   = "yellow"     # default: yellow
```

Values accept any form both tools take: a named color (`red`, `magenta`, `cyan`, …), a 256-color index (`214` or `colour214`), or hex (`#ff8800`).

### Popup size

By default the list pickers (`fr`, `sybau`, `pbqt`, `wth`, `wtf`) grow their popup to fit every option, which gets tall and thin with a long list. `max-options` caps how many rows the popup is sized for; fzf scrolls through the rest and shows its `matched/total` count so you can tell more options exist.

```toml
max-options = 10   # omit (or 0) to grow the popup to fit every option
```

The cap applies only to the tmux popup. Outside tmux the pickers fall back to inline fzf, which manages its own height.

## Recipes

Each recipe is a TOML file in the recipe directory. The filename becomes the session name.

```toml
# ~/.config/twin/recipes/front.toml
start-directory = "~/Developer/pro/my-frontend/"

[[windows]]
name = "editor"
start-directory = "src/"
command = "nvim"

[[windows]]
# empty window — just a shell

[[windows]]
command = "lazygit"

[[windows]]
# commands = [...] runs each entry as a separate shell line
commands = ["npm install", "make run"]
```

- `start-directory` (top-level) is required
- `start-directory` on a window is relative to the recipe's top-level directory
- `name = "x"` gives the window a fixed name; without it tmux shows the running
  command (naming a window disables tmux's automatic-rename for it)
- `command = "x"` runs a single command; omit it for a plain shell
- `commands = ["x", "y"]` runs each entry as its own shell line — handy for a short
  setup sequence (or just chain with `&&`). Use `command` **or** `commands`, not both
- `~` and environment variables are expanded in paths

### Panes

A window can be split into multiple panes instead of holding a single shell:

```toml
[[windows]]
  [[windows.panes]]
  title = "vim"                # optional, shown in the pane border
  command = "nvim"             # pane 1 — the window's base pane

  [[windows.panes]]
  split = "right"              # new pane to the right of pane 1
  size  = "30%"
  command = "lazygit"

  [[windows.panes]]
  split = "down"               # splits the previous pane
  size  = "50%"
  start-directory = "logs/"
  command = "tail -f app.log"
  focus = true                 # focus this pane when the window opens
```

- pane 1 is the base pane; later panes split an earlier one
- `split` is `"right"` (side-by-side) or `"down"` (stacked); defaults to `"right"`
- `split-from = <N>` (1-based) chooses which pane to split; omit it to split the previous pane
- `size` is a percentage like `"30%"`, optional — it's relative to the pane being split (needs tmux ≥3.1)
- `command`/`commands`, `start-directory`, and `focus = true` (one per window) work per-pane
- `title = "x"` titles a pane; when any pane in a window has one, twin turns on
  `pane-border-status top` for that window so the titles are actually visible.
  Programs that set the terminal title (via escape sequences) can overwrite a
  pane title later — twin sets it once at creation
- a window uses **either** `command`/`commands` or `panes`, never both

### Twins

A twin is a second copy of a recipe's repo — a git worktree at `<repo>.twins/<name>/`
with its own session `<recipe>@<name>` (see [`twin wth`](#twin-wth)). Git only checks
out tracked files, so a `[worktree]` block says what else a fresh twin needs. Put the
shared part in `twin.toml` and recipe-specific bits in the recipe; the lists are appended:

```toml
# twin.toml
[worktree]
copy = [".envrc", ".env", "jdh", ".claude/settings.local.json"]

# recipes/front.toml
[[windows]]
command = "npm run dev"
skip-in-wt = true              # a twin opens this window as a plain shell

[worktree]
copy  = ["apps/api/.env"]
setup = ["npm install"]
```

- `copy` entries are paths from the repo root, copied over from the main checkout.
  Missing ones are skipped silently, ones git already tracks are left alone
- if an `.envrc` was copied, twin runs `direnv allow` on it and loads it once; `setup` commands run
  inside that environment
- `setup` runs once, in the new worktree, before its session is built. If a command
  fails (or you Ctrl-C it), the whole hatch is rolled back
- `skip-in-wt = true` on a window or pane drops its command(s) in twins — for dev
  servers and workers that would fight the original over ports or a database

## Subcommands

### `twin tspmo`

> Tmux Session Project Management Opener

Spin up tmux sessions for all `active` recipes. Skips sessions that already exist.

### `twin fr`

> From Recipe

Open a single recipe session.

```sh
twin fr              # fzf picker (only shows unopened recipes)
twin fr myproject    # open a specific recipe
twin fr --list       # list all available recipes
```

The picker also offers twins whose session isn't running (after a reboot, say), as
`<recipe>@<name>`.

### `twin sybau`

> Switch Your Basis of Active Undertakement

Fuzzy session switcher in a tmux popup. Bind it in `tmux.conf`:

```
bind-key Space run-shell "twin sybau --preview"
```

Twins are listed right under their recipe's session.

### `twin wth`

> Worktree Hatch

Open a twin: a git worktree of a recipe's repo with its own session, so you can work on
two or three things side by side — each with its own editor, shells and Claude.

```sh
twin wth blue                    # twin "blue" of the current session's recipe
twin wth front@blue feat/42-x    # name the recipe and the branch
twin wth                         # pick or type the name, then the branch
```

- `<name>` is the twin's id: its folder is `<repo>.twins/<name>/` next to the main
  checkout, its session `<recipe>@<name>`
- name and branch are picked in fzf when omitted (a popup inside tmux). Pick an
  existing one or type a new one; alt-enter takes what you typed over a fuzzy match
- a new branch starts from origin's default branch, after a `git fetch`
- run it on an existing twin and it just (re)opens the session
- `--no-attach` skips switching to it

### `twin wtf`

> Worktree Fold

Remove a twin: its worktree, its session, and its branch when that loses nothing.

```sh
twin wtf blue          # fold twin "blue" of the current session's recipe
twin wtf               # pick it in fzf
```

- refuses when the worktree has modified or untracked files, or commits that are on
  no branch, so nothing gets lost
- the branch is deleted only if its upstream is gone (a merged and deleted PR branch)
  or it has no commits beyond origin's default branch; otherwise it's kept
- folding the twin you're in drops you on the recipe's own session

## Disclaimer

This project is entirely vibecoded. Virtually none of the code is written by me — it's all Claude.
