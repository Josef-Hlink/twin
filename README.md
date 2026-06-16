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

Each subcommand that uses a popup picker (`fr`, `sybau`, `pbqt`, `icl`) has a configurable accent color that paints both the tmux popup border and fzf's pointer/marker, so the picker reads as one themed surface.

```toml
fr-border-color    = "green"      # default: green
sybau-border-color = "magenta"    # default: magenta
pbqt-border-color  = "red"        # default: red
icl-border-color   = "colour214"  # default: colour214 (orange)
```

Values accept any form both tools take: a named color (`red`, `magenta`, `cyan`, …), a 256-color index (`214` or `colour214`), or hex (`#ff8800`).

## Recipes

Each recipe is a TOML file in the recipe directory. The filename becomes the session name.

```toml
# ~/.config/twin/recipes/front.toml
start-directory = "~/Developer/pro/my-frontend/"

[[windows]]
start-directory = "src/"
commands = ["nvim"]

[[windows]]
# empty window — just a shell

[[windows]]
commands = ["lazygit"]

[[windows]]
commands = ["make run"]
```

- `start-directory` (top-level) is required
- `start-directory` on a window is relative to the recipe's top-level directory
- `commands` is optional; omit it for a plain shell
- `~` and environment variables are expanded in paths

### Panes

A window can be split into multiple panes instead of holding a single shell:

```toml
[[windows]]
  [[windows.panes]]
  commands = ["nvim"]          # pane 1 — the window's base pane

  [[windows.panes]]
  split = "right"              # new pane to the right of pane 1
  size  = "30%"
  commands = ["lazygit"]

  [[windows.panes]]
  split = "down"               # splits the previous pane
  size  = "50%"
  start-directory = "logs/"
  commands = ["tail -f app.log"]
  focus = true                 # focus this pane when the window opens
```

- pane 1 is the base pane; later panes split an earlier one
- `split` is `"right"` (side-by-side) or `"down"` (stacked); defaults to `"right"`
- `split-from = <N>` (1-based) chooses which pane to split; omit it to split the previous pane
- `size` is a percentage like `"30%"`, optional — it's relative to the pane being split (needs tmux ≥3.1)
- `commands`, `start-directory`, and `focus = true` (one per window) work per-pane
- a window uses **either** window-level `commands` or `panes`, never both

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

### `twin sybau`

> Switch Your Basis of Active Undertakement

Fuzzy session switcher in a tmux popup. Bind it in `tmux.conf`:

```
bind-key Space run-shell "twin sybau --preview"
```

## Disclaimer

This project is entirely vibecoded. Virtually none of the code is written by me — it's all Claude.
