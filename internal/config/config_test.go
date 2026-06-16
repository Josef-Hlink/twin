package config

import "testing"

func TestRecipeValidate(t *testing.T) {
	tests := []struct {
		name    string
		recipe  Recipe
		wantErr bool
	}{
		{
			name:   "empty recipe",
			recipe: Recipe{},
		},
		{
			name: "window with commands only",
			recipe: Recipe{Windows: []Window{
				{Commands: []string{"nvim"}},
			}},
		},
		{
			name: "valid pane tree",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "right", Size: "30%", Commands: []string{"lazygit"}},
					{Split: "down", Size: "50%", Focus: true},
				}},
			}},
		},
		{
			name: "split-from referencing earlier pane",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "right"},
					{SplitFrom: 1, Split: "down"},
				}},
			}},
		},
		{
			name: "commands and panes both set",
			recipe: Recipe{Windows: []Window{
				{Commands: []string{"nvim"}, Panes: []Pane{{Commands: []string{"ls"}}}},
			}},
			wantErr: true,
		},
		{
			name: "base pane sets split",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{{Split: "right"}}},
			}},
			wantErr: true,
		},
		{
			name: "split-from out of range",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{SplitFrom: 5},
				}},
			}},
			wantErr: true,
		},
		{
			name: "invalid split direction",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "sideways"},
				}},
			}},
			wantErr: true,
		},
		{
			name: "invalid size",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}},
					{Split: "right", Size: "30"},
				}},
			}},
			wantErr: true,
		},
		{
			name: "two focused panes",
			recipe: Recipe{Windows: []Window{
				{Panes: []Pane{
					{Commands: []string{"nvim"}, Focus: true},
					{Split: "right", Focus: true},
				}},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.recipe.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
