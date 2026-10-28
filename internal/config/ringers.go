package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Ringers is the [ringers] block of twin.toml. A ringer is a named copy of a
// recipe's session (<recipe>@<name>); twin builds and tracks the session, and
// these commands make the copy itself real, however the user's setup does
// that (a clone, a container, a remote box...).
type Ringers struct {
	Names       []string          `toml:"names"`        // the names ring offers; empty allows any
	Dir         string            `toml:"dir"`          // where a ringer's session starts; must use {name}
	Create      string            `toml:"create"`       // makes a new ringer; must leave Dir behind
	Remove      string            `toml:"remove"`       // drops one; a non-zero exit keeps it
	ForceRemove string            `toml:"force-remove"` // optional: what unring --force runs instead of Remove
	Enter       string            `toml:"enter"`        // optional: every shell in a ringer starts through it
	Main        string            `toml:"main"`         // optional: same, for the recipe's own session
	Colors      map[string]string `toml:"colors"`       // optional: tint per ringer name
}

// Vars are the values a [ringers] template can refer to as {recipe}, {name},
// {branch}, {repo} and {dir}. Empty ones expand to nothing.
type Vars struct {
	Recipe string // the recipe's name
	Name   string // the ringer's name
	Branch string // the branch asked for, if any
	Repo   string // the recipe's start-directory
}

// Configured reports whether twin.toml has a [ringers] block that ring can
// use: somewhere for ringers to live and a way to create one.
func (r Ringers) Configured() bool {
	return r.Dir != "" && r.Create != ""
}

// Validate checks the [ringers] block before anything runs.
func (r Ringers) Validate() error {
	if !r.Configured() {
		return fmt.Errorf("twin.toml has no [ringers] block with dir and create")
	}
	if n := strings.Count(r.Dir, "{name}"); n != 1 {
		return fmt.Errorf("[ringers] dir must contain {name} exactly once, got %q", r.Dir)
	}
	if strings.Contains(r.Dir, "{dir}") || strings.Contains(r.Dir, "{branch}") {
		return fmt.Errorf("[ringers] dir can't use {dir} or {branch}")
	}
	return nil
}

// Allows reports whether name is one of the configured names (any name is
// allowed when none are configured).
func (r Ringers) Allows(name string) bool {
	return len(r.Names) == 0 || slices.Contains(r.Names, name)
}

// Color returns the tint for a ringer's session, or "" for none.
func (r Ringers) Color(name string) Color {
	return Color(r.Colors[name])
}

// DirFor expands the dir template into the ringer's absolute directory.
// Placeholders go in as-is (no shell quoting: this is a path, not a command),
// then a leading ~ and $VARS are expanded.
func (r Ringers) DirFor(v Vars) string {
	return ExpandPath(expand(r.Dir, v.values(""), false))
}

// Command expands a command template (create, remove, force-remove, enter)
// for one ringer.
// {dir} is the ringer's expanded directory.
func (r Ringers) Command(tmpl string, v Vars) string {
	return expand(tmpl, v.values(r.DirFor(v)), true)
}

// MainCommand expands main for a recipe's own session. Only {recipe} and
// {repo} mean anything there; {name}, {branch} and {dir} expand to nothing.
// Returns "" when main isn't set.
func (r Ringers) MainCommand(recipe, repo string) string {
	return expand(r.Main, Vars{Recipe: recipe, Repo: repo}.values(""), true)
}

// DirPattern splits the dir template around {name}, with the other
// placeholders expanded, so the existing ringers of a recipe can be found by
// globbing for prefix + "*" + suffix. Validate guarantees {name} is there.
func (r Ringers) DirPattern(recipe, repo string) (prefix, suffix string) {
	const mark = "\x00"
	v := Vars{Recipe: recipe, Name: mark, Repo: repo}
	full := r.DirFor(v)
	prefix, suffix, _ = strings.Cut(full, mark)
	return prefix, suffix
}

func (v Vars) values(dir string) map[string]string {
	return map[string]string{
		"{recipe}": v.Recipe,
		"{name}":   v.Name,
		"{branch}": v.Branch,
		"{repo}":   v.Repo,
		"{dir}":    dir,
	}
}

// expand replaces each {placeholder} in tmpl. With quote, values are
// shell-quoted when they need it, and an empty value disappears instead of
// becoming an empty argument (so `create {name} {branch}` without a branch
// just ends after the name).
func expand(tmpl string, vals map[string]string, quote bool) string {
	var pairs []string
	for k, v := range vals {
		if quote {
			v = shellQuote(v)
		}
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}

// shellQuote wraps s in single quotes unless it's made only of characters no
// shell treats specially. "" stays "".
func shellQuote(s string) string {
	if s == "" {
		return ""
	}
	safe := true
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			strings.ContainsRune("@%+=:,./_-", c)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ForRinger returns a copy of the recipe rooted at dir, for a ringer's
// session. Windows and panes marked skip-in-ringers lose their commands, so
// they open as plain shells, for setups where the copies can't run the same
// dev servers side by side.
func (r Recipe) ForRinger(dir string) Recipe {
	t := r
	t.StartDirectory = filepath.Clean(dir)
	t.Windows = make([]Window, len(r.Windows))
	for i, w := range r.Windows {
		// w is a copy of the window, but w.Panes still shares its backing
		// array with the original recipe, so clone it before editing.
		w.Panes = slices.Clone(w.Panes)
		if w.SkipInRingers {
			w.Command, w.Commands = "", nil
		}
		for j := range w.Panes {
			if w.SkipInRingers || w.Panes[j].SkipInRingers {
				w.Panes[j].Command, w.Panes[j].Commands = "", nil
			}
		}
		t.Windows[i] = w
	}
	return t
}
