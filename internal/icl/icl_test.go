package icl

import (
	"reflect"
	"testing"
)

func TestLayoutTabs(t *testing.T) {
	tabs := []tab{
		{text: " a:1 - "},           // 7 cols
		{text: " pylona@red:5 - "},  // 16 cols
		{text: " twin:1 * "},        // 10 cols
		{text: " pylona@blue:1 & "}, // 17 cols
	}
	names := func(rows [][]tab) [][]string {
		var out [][]string
		for _, row := range rows {
			var r []string
			for _, t := range row {
				r = append(r, t.text)
			}
			out = append(out, r)
		}
		return out
	}

	cases := []struct {
		width  int
		prefix string
		want   [][]string
	}{
		{100, "", [][]string{{" a:1 - ", " pylona@red:5 - ", " twin:1 * ", " pylona@blue:1 & "}}},
		{24, "", [][]string{{" a:1 - ", " pylona@red:5 - "}, {" twin:1 * "}, {" pylona@blue:1 & "}}}, // 7+1+16=24 fits
		{35, "", [][]string{{" a:1 - ", " pylona@red:5 - ", " twin:1 * "}, {" pylona@blue:1 & "}}},
		{5, "", [][]string{{" a:1 - "}, {" pylona@red:5 - "}, {" twin:1 * "}, {" pylona@blue:1 & "}}},
		{25, "xx", [][]string{{"xx a:1 - "}, {" pylona@red:5 - "}, {" twin:1 * "}, {" pylona@blue:1 & "}}}, // prefix counts
	}
	for _, c := range cases {
		got := names(layoutTabs(tabs, c.width, c.prefix))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("width %d prefix %q: got %v, want %v", c.width, c.prefix, got, c.want)
		}
	}
}

func TestVisibleWidth(t *testing.T) {
	if got := visibleWidth("\033[7m ab \033[0m\033[38;5;214m|"); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}
