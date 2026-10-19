# twin

A lightweight CLI tool for managing tmux sessions from TOML recipes.

Born out of the frustration of manually setting up tmux workspaces every morning.

## Requirements

- Go 1.24+
- [tmux](https://github.com/tmux/tmux)
- [fzf](https://github.com/junegunn/fzf)

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

Each subcommand that uses a popup picker (`fr`, `sybau`, `pbqt`, `icl`, `ring`, `unring`) has a configurable accent color that paints both the tmux popup border and fzf's pointer/marker, so the picker reads as one themed surface.

```toml
fr-border-color    = "green"      # default: green
sybau-border-color = "magenta"    # default: magenta
pbqt-border-color  = "red"        # default: red
icl-border-color   = "colour214"  # default: colour214 (orange)
ring-border-color   = "blue"      # default: blue
unring-border-color = "yellow"    # default: yellow
```

Values accept any form both tools take: a named color (`red`, `magenta`, `cyan`, …), a 256-color index (`214` or `colour214`), or hex (`#ff8800`).

### Popup size

By default the list pickers (`fr`, `sybau`, `pbqt`, `ring`, `unring`) grow their popup to fit every option, which gets tall and thin with a long list. `max-options` caps how many rows the popup is sized for; fzf scrolls through the rest and shows its `matched/total` count so you can tell more options exist.

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

## Ringers

A ringer is a named copy of a recipe's session, `<recipe>@<name>`, so you can work on
two or three things side by side — each with its own editor, shells and Claude. twin
owns the session: naming it, building it from the recipe in the ringer's directory,
picking it, listing it in `sybau` and `fr`, tinting it. What makes the copy itself real
— a clone, a container, a remote box — is up to the commands you configure in
`twin.toml`:

```toml
[ringers]
names  = ["red", "green", "blue"]      # what `ring` offers; omit to allow any name
dir    = "~/copies/{name}/{recipe}"     # where a ringer's session starts
create = "mkcopy {name} {recipe} {branch}"
remove = "rmcopy {name} {recipe}"
enter  = "copy-shell {name}"            # optional: every shell in the ringer starts through this
main   = "copy-shell default {recipe}"  # optional: same, for the recipe's own session

[ringers.colors]                         # optional: tint a ringer's session by name
red = "red"
```

- placeholders: `{recipe}`, `{name}`, `{branch}`, `{repo}` (the recipe's start-directory)
  and `{dir}` (the expanded `dir`). In commands, values are shell-quoted when they need
  it and an empty one (no branch given) disappears. In `main` only `{recipe}` and
  `{repo}` mean anything
- `dir` must contain `{name}` once; a ringer exists when its `dir` does
- `create` must leave `dir` behind; a non-zero exit means no ringer
- `remove` decides what's safe to delete: a non-zero exit keeps the ringer, so put any
  "is there unpushed work?" check in there
- `enter` becomes the session's tmux `default-command`, so every window and pane —
  including ones you split by hand later — starts inside the ringer
- `skip-in-ringers = true` on a recipe window or pane opens it as a plain shell in
  ringers, for setups where copies can't run the dev servers side by side

The simplest setup is plain git worktrees. They share your ports and databases, so
servers get `skip-in-ringers`:

```toml
[ringers]
names  = ["a", "b", "c"]
dir    = "{repo}.ringers/{name}"
create = "git -C {repo} worktree add {dir} {branch}"
remove = "git -C {repo} worktree remove {dir}"
```

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

The picker also offers ringers whose session isn't running (after a reboot, say), as
`<recipe>@<name>`.

### `twin sybau`

> Switch Your Basis of Active Undertakement

Fuzzy session switcher in a tmux popup. Bind it in `tmux.conf`:

```
bind-key Space run-shell "twin sybau --preview"
```

Ringers are listed right under their recipe's session.

### `twin ring`

Open a ringer (see [Ringers](#ringers)). Unlike the others, `ring` and `unring` are
plain words, not slang.

```sh
twin ring red                    # ringer "red" of the current session's recipe
twin ring front@red feat/42-x    # name the recipe, and a branch for create
twin ring                        # pick the name (and the branch, when create uses one)
```

- a ringer whose `dir` exists just gets its session (re)opened
- a new one runs `create` first, in the popup, so its output shows
- name and branch are picked in fzf when omitted; the branches come from `{repo}` when
  it's a git repo, and typing a new one works too (alt-enter takes the typed text over
  a fuzzy match). With `names` set, only those names are accepted
- `--no-attach` skips switching to it

### `twin unring`

Drop a ringer: runs `remove`, then kills its session. If you're in it, you land on the
recipe's own session.

```sh
twin unring red        # drop ringer "red" of the current session's recipe
twin unring            # pick it in fzf
```

## Disclaimer

This project is entirely vibecoded. Virtually none of the code is written by me — it's all Claude.
