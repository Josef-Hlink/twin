package ringers

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Josef-Hlink/twin/internal/config"
)

func TestSessionNames(t *testing.T) {
	if got := SessionName("pylona", "red"); got != "pylona@red" {
		t.Fatalf("SessionName() = %q", got)
	}
	if r, n, ok := SplitSession("pylona@red"); !ok || r != "pylona" || n != "red" {
		t.Fatalf("SplitSession(pylona@red) = %q, %q, %v", r, n, ok)
	}
	if _, _, ok := SplitSession("pylona"); ok {
		t.Fatal("a recipe session isn't a ringer")
	}
	if RecipeOf("pylona@red") != "pylona" || RecipeOf("dots") != "dots" {
		t.Fatal("RecipeOf()")
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"red", "613", "feat-42", "a_b"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-x", "a/b", "a.b", "a:b", "a@b", "a b", "it's"} {
		if ValidateName(bad) == nil {
			t.Errorf("ValidateName(%q) accepted", bad)
		}
	}
}

func TestParseTarget(t *testing.T) {
	t.Setenv("TMUX", "") // outside tmux: the recipe must be spelled out

	if r, n, err := ParseTarget("pylona@red"); err != nil || r != "pylona" || n != "red" {
		t.Fatalf("ParseTarget(pylona@red) = %q, %q, %v", r, n, err)
	}
	if r, n, err := ParseTarget("pylona@"); err != nil || r != "pylona" || n != "" {
		t.Fatalf("ParseTarget(pylona@) = %q, %q, %v (empty name = pick it)", r, n, err)
	}
	if _, _, err := ParseTarget("@red"); err == nil {
		t.Fatal("ParseTarget(@red) accepted a missing recipe")
	}
	if _, _, err := ParseTarget("red"); err == nil {
		t.Fatal("ParseTarget(red) outside tmux should ask for the recipe")
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"red/pylona", "blue/pylona", "green/klym", "has.dot/pylona"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	recipe := config.Recipe{StartDirectory: "/src/pylona"}

	// With names: those whose dir exists, in configured order.
	cfg := config.Config{Ringers: config.Ringers{
		Names:  []string{"green", "blue", "red"},
		Dir:    root + "/{name}/{recipe}",
		Create: "x",
	}}
	if got, want := List(cfg, "pylona", recipe), []string{"blue", "red"}; !slices.Equal(got, want) {
		t.Fatalf("List() with names = %v, want %v", got, want)
	}
	if got, want := Free(cfg, []string{"blue", "red"}), []string{"green"}; !slices.Equal(got, want) {
		t.Fatalf("Free() = %v, want %v", got, want)
	}

	// Without names: whatever matches the dir template (and is a valid name).
	cfg.Ringers.Names = nil
	if got, want := List(cfg, "pylona", recipe), []string{"blue", "red"}; !slices.Equal(got, want) {
		t.Fatalf("List() without names = %v, want %v", got, want)
	}
}
