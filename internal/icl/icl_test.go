package icl

import (
	"reflect"
	"testing"
)

func TestLayoutTabs(t *testing.T) {
	panes := []claudePane{
		{label: "a:1", state: '-'},           // " a:1 - " = 7 cols
		{label: "pylona@red:5", state: '-'},  // 16 cols
		{label: "twin:1", state: '*'},        // 10 cols
		{label: "pylona@blue:1", state: '&'}, // 17 cols
	}

	cases := []struct {
		width int
		want  [][]int
	}{
		{width: 100, want: [][]int{{0, 1, 2, 3}}},
		{width: 24, want: [][]int{{0, 1}, {2}, {3}}},  // 7+1+16=24 fits, +1+10 doesn't
		{width: 35, want: [][]int{{0, 1, 2}, {3}}},    // 7+1+16+1+10=35 fits
		{width: 5, want: [][]int{{0}, {1}, {2}, {3}}}, // too narrow: one per row
	}
	for _, c := range cases {
		got := layoutTabs(panes, c.width)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("width %d: got %v, want %v", c.width, got, c.want)
		}
	}
}
