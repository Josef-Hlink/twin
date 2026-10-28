package unring

import (
	"testing"

	"github.com/Josef-Hlink/twin/internal/config"
)

func TestParse(t *testing.T) {
	t.Setenv("TMUX", "") // outside tmux: the recipe must be spelled out

	o, err := parse([]string{"pylona@red"})
	if err != nil || o.recipe != "pylona" || o.name != "red" || o.force {
		t.Fatalf("parse(pylona@red) = %+v, %v", o, err)
	}
	// --force goes before or after the target.
	for _, args := range [][]string{{"--force", "pylona@red"}, {"pylona@red", "--force"}} {
		o, err := parse(args)
		if err != nil || o.recipe != "pylona" || o.name != "red" || !o.force {
			t.Fatalf("parse(%q) = %+v, %v", args, o, err)
		}
	}
	if _, err := parse([]string{"pylona@red", "--forse"}); err == nil {
		t.Fatal("parse accepted an unknown flag")
	}
	if _, err := parse([]string{"pylona@red", "pylona@blue"}); err == nil {
		t.Fatal("parse accepted two targets")
	}
}

func TestRemoveCommand(t *testing.T) {
	r := config.Ringers{Remove: "rmcopy {name}", ForceRemove: "rmcopy {name} --force"}

	if tmpl, key, err := removeCommand(r, false); err != nil || tmpl != r.Remove || key != "remove" {
		t.Fatalf("removeCommand(force=false) = %q, %q, %v", tmpl, key, err)
	}
	if tmpl, key, err := removeCommand(r, true); err != nil || tmpl != r.ForceRemove || key != "force-remove" {
		t.Fatalf("removeCommand(force=true) = %q, %q, %v", tmpl, key, err)
	}

	// --force never falls back to remove: that one may refuse, which is
	// exactly what --force is for.
	r.ForceRemove = ""
	if _, _, err := removeCommand(r, true); err == nil {
		t.Fatal("removeCommand(force=true) without force-remove should fail")
	}
	if _, _, err := removeCommand(config.Ringers{}, false); err == nil {
		t.Fatal("removeCommand(force=false) without remove should fail")
	}
}
