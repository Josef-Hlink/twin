package twins

import "testing"

func TestValidateName(t *testing.T) {
	for _, name := range []string{"blue", "62", "fix-login", "a_b"} {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want ok", name, err)
		}
	}
	for _, name := range []string{"", "-x", "feat/x", "v1.2", "a:b", "a@b", "two words", "tab\tname"} {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want an error", name)
		}
	}
}

func TestSessionNames(t *testing.T) {
	s := SessionName("pylona", "blue")
	if s != "pylona@blue" {
		t.Fatalf("SessionName = %q", s)
	}
	if recipe, name, ok := SplitSession(s); !ok || recipe != "pylona" || name != "blue" {
		t.Fatalf("SplitSession(%q) = %q, %q, %v", s, recipe, name, ok)
	}
	if _, _, ok := SplitSession("pylona"); ok {
		t.Fatal("SplitSession treated a recipe session as a twin")
	}
	if RecipeOf(s) != "pylona" || RecipeOf("pylona") != "pylona" {
		t.Fatalf("RecipeOf: %q, %q", RecipeOf(s), RecipeOf("pylona"))
	}
}
