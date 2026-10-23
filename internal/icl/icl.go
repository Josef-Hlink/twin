package icl

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/ringers"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

// claudePane wraps a tmux pane with cached content and detected state.
type claudePane struct {
	pane    tmux.Pane
	label   string // "session:window"
	content string // captured pane output
	state   byte   // '*' busy, '&' menu, '-' idle
}

const Usage = `usage: twin icl

interactive tabbed quickview of running claude agent panes, grouped by
recipe: a recipe's own session and its ringers share one tab.
  H / left      previous recipe
  L / right     next recipe
  J / down      next session of the recipe
  K / up        previous session of the recipe
  za            toggle the fold: a second row listing the recipe's
                sessions, or the selected recipe inflated inline
  Enter         switch to the selected pane
  q / Esc       close
`

// Run spawns a centered tmux popup showing the interactive Claude quickview.
func Run() error {
	panes, err := findClaudePanes()
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		fmt.Println("no claude panes found")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	clientW, clientH, _ := tmux.ClientSize()
	width := clientW * 80 / 100
	if width < 40 {
		width = 40
	}
	height := clientH * 70 / 100
	if height < 10 {
		height = 10
	}

	return popup.LaunchCenter("icl", width, height, "icl-view", cfg.BorderColor("icl"))
}

// RunView renders the interactive tabbed Claude pane viewer. Runs inside the popup.
func RunView() error {
	panes, err := findClaudePanes()
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		fmt.Println("no claude panes found")
		return nil
	}

	// Build claudePane list with captured content and state.
	cPanes := make([]claudePane, len(panes))
	for i, p := range panes {
		label := fmt.Sprintf("%s:%d", p.SessionName, p.WindowIndex)
		content, _ := tmux.CapturePane(p.Target)
		cPanes[i] = claudePane{
			pane:    p,
			label:   label,
			content: content,
			state:   detectState(content),
		}
	}

	// Get popup terminal size via stty.
	width, height := termSize()
	if width == 0 || height == 0 {
		width, height = 80, 24
	}

	// Enter raw mode.
	restore, err := sttyRaw()
	if err != nil {
		return fmt.Errorf("entering raw mode: %w", err)
	}
	defer restore()

	v := &view{groups: groupPanes(cPanes), folded: true}
	buf := make([]byte, 16)
	pendingZ := false

	for {
		render(os.Stdout, cPanes, v, width, height)

		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		// One read may hold several keys (pasted or sent at once), so walk
		// them all. Arrow keys arrive as ESC [ A-D and map onto hjkl.
		for i := 0; i < n; i++ {
			key := buf[i]
			if key == 27 {
				if i+2 >= n || buf[i+1] != '[' {
					return nil // a bare Escape closes
				}
				key = map[byte]byte{'A': 'k', 'B': 'j', 'C': 'l', 'D': 'h'}[buf[i+2]]
				i += 2
			}

			// z is a prefix (vim's fold keys): only za does something.
			if pendingZ {
				pendingZ = false
				if key == 'a' {
					v.folded = !v.folded
				}
				continue
			}

			switch key {
			case 'q':
				return nil
			case 13: // Enter: switch to the selected pane
				p := cPanes[v.current()].pane
				target := fmt.Sprintf("%s:%d", p.SessionName, p.WindowIndex)
				tmux.SwitchClient(p.SessionName)
				tmux.SelectWindow(target)
				return nil
			case 'z':
				pendingZ = true
			case 'h', 'H': // previous recipe
				v.moveGroup(-1)
			case 'l', 'L': // next recipe
				v.moveGroup(1)
			case 'j', 'J': // next session of the recipe
				v.moveMember(1)
			case 'k', 'K': // previous session of the recipe
				v.moveMember(-1)
			}
		}
	}

	return nil
}

// --- Grouping and selection ---

// group is one recipe's tab: its own session (if open) and its ringers, as
// indices into the pane list. The recipe's own session sorts first.
type group struct {
	recipe  string
	members []int
	cursor  int // the selected member, remembered across tab switches
}

// view is the state the viewer navigates: which recipe, which of its
// sessions, and whether the ringers are folded away under the recipe tab.
type view struct {
	groups   []group
	selected int
	folded   bool
}

// groupPanes groups the panes by recipe, keeping the panes' order (tmux lists
// sessions alphabetically, so a recipe's own session comes before its ringers).
func groupPanes(panes []claudePane) []group {
	var groups []group
	index := make(map[string]int)
	for i, p := range panes {
		recipe := ringers.RecipeOf(p.pane.SessionName)
		g, ok := index[recipe]
		if !ok {
			g = len(groups)
			index[recipe] = g
			groups = append(groups, group{recipe: recipe})
		}
		groups[g].members = append(groups[g].members, i)
	}
	return groups
}

func (v *view) current() int {
	g := v.groups[v.selected]
	return g.members[g.cursor]
}

func (v *view) moveGroup(d int) {
	v.selected = (v.selected + d + len(v.groups)) % len(v.groups)
}

func (v *view) moveMember(d int) {
	g := &v.groups[v.selected]
	g.cursor = (g.cursor + d + len(g.members)) % len(g.members)
}

// hasRingers reports whether the group holds anything but the recipe's own
// session: a group of one plain session shows as a single tab.
func (g group) hasRingers(panes []claudePane) bool {
	if len(g.members) > 1 {
		return true
	}
	_, _, isRinger := ringers.SplitSession(panes[g.members[0]].pane.SessionName)
	return isRinger
}

// worstState picks the most urgent state among the group's sessions:
// a menu waiting beats busy beats idle.
func (g group) worstState(panes []claudePane) byte {
	rank := map[byte]int{'-': 0, '*': 1, '&': 2}
	worst := byte('-')
	for _, i := range g.members {
		if rank[panes[i].state] > rank[worst] {
			worst = panes[i].state
		}
	}
	return worst
}

// --- Terminal helpers ---

// sttyRaw puts the terminal in raw mode and returns a function to restore it.
func sttyRaw() (restore func(), err error) {
	// Save current settings. Must connect stdin so stty sees the terminal.
	save := exec.Command("stty", "-g")
	save.Stdin = os.Stdin
	out, err := save.Output()
	if err != nil {
		return nil, err
	}
	saved := strings.TrimSpace(string(out))

	// Enter raw mode.
	raw := exec.Command("stty", "raw", "-echo")
	raw.Stdin = os.Stdin
	if err := raw.Run(); err != nil {
		return nil, err
	}

	return func() {
		cmd := exec.Command("stty", saved)
		cmd.Stdin = os.Stdin
		cmd.Run()
	}, nil
}

// termSize returns terminal cols and rows via stty size.
func termSize() (width, height int) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return 0, 0
	}
	h, _ := strconv.Atoi(fields[0])
	w, _ := strconv.Atoi(fields[1])
	return w, h
}

// --- State detection ---

var menuLineRe = regexp.MustCompile(`^\s*\d+\.\s`)

// detectState scans the last ~20 lines of captured content to determine pane state.
func detectState(content string) byte {
	lines := lastLines(content, 20)

	// Check last 10 lines for menu pattern (2+ matches = menu).
	scanLines := lines
	if len(scanLines) > 10 {
		scanLines = scanLines[len(scanLines)-10:]
	}
	menuCount := 0
	for _, line := range scanLines {
		if isMenuLine(line) {
			menuCount++
		}
	}
	if menuCount >= 2 {
		return '&'
	}

	// Check all 20 lines for busy pattern.
	for i := len(lines) - 1; i >= 0; i-- {
		if isBusyLine(lines[i]) {
			return '*'
		}
	}

	return '-'
}

// isMenuLine checks if a line matches the "N. " pattern (digit, dot, space).
func isMenuLine(line string) bool {
	return menuLineRe.MatchString(line)
}

// isBusyLine checks if a line contains a word ending in "ing…" or "ing..."
// which matches Claude's spinner status text (Crunching…, Reading…, etc.).
func isBusyLine(line string) bool {
	for w := range strings.FieldsSeq(line) {
		lower := strings.ToLower(w)
		if strings.HasSuffix(lower, "ing…") || strings.HasSuffix(lower, "ing...") {
			return true
		}
	}
	return false
}

// --- Rendering ---

const (
	ansiOrange  = "\033[38;5;214m"
	ansiReverse = "\033[7m"
	ansiReset   = "\033[0m"
	ansiClear   = "\033[2J\033[H" // clear screen + cursor home
	ansiDim     = "\033[2m"
)

// tab is one cell of a tab row.
type tab struct {
	text     string
	selected bool
}

// render draws the full TUI: tab rows, separator, preview, status line.
func render(w io.Writer, panes []claudePane, v *view, width, height int) {
	var b strings.Builder

	// Clear screen and move cursor home.
	b.WriteString(ansiClear)

	// --- Tab rows ---
	var rows [][]tab
	if v.folded {
		rows = layoutTabs(recipeTabs(panes, v, false), width, "")
		rows = append(rows, layoutTabs(memberTabs(panes, v.groups[v.selected]), width, "  ↳")...)
	} else {
		rows = layoutTabs(recipeTabs(panes, v, true), width, "")
	}
	for _, row := range rows {
		b.WriteString(ansiOrange)
		var line strings.Builder
		for j, t := range row {
			if t.selected {
				line.WriteString(ansiReverse)
				line.WriteString(t.text)
				line.WriteString(ansiReset)
				line.WriteString(ansiOrange)
			} else {
				line.WriteString(t.text)
			}
			if j < len(row)-1 {
				line.WriteString("|")
			}
		}
		b.WriteString(truncVisible(line.String(), width))
		b.WriteString(ansiReset)
		b.WriteString("\r\n")
	}

	// --- Separator ---
	b.WriteString(ansiOrange)
	b.WriteString(strings.Repeat("━", width))
	b.WriteString(ansiReset)
	b.WriteString("\r\n")

	// --- Preview area (below the separator, above the status line) ---
	previewHeight := max(height-len(rows)-2, 1) // tab rows + separator + status line

	content := panes[v.current()].content
	previewLines := lastLines(content, previewHeight)

	for i := range previewHeight {
		if i < len(previewLines) {
			line := truncVisible(previewLines[i], width)
			b.WriteString(line)
			b.WriteString(ansiReset) // prevent color bleed across lines
		}
		b.WriteString("\r\n")
	}

	// --- Status line (last line) ---
	b.WriteString(ansiDim)
	status := " H/L: recipe  J/K: session  za: fold  Enter: switch  q: close"
	b.WriteString(truncVisible(status, width))
	b.WriteString(ansiReset)

	w.Write([]byte(b.String()))
}

// recipeTabs builds the recipe row: " recipe state " for a lone session,
// " recipe N state " for a recipe with ringers (N sessions, worst state).
// Inflated, the selected recipe instead spells out its sessions inline and
// the selection sits on one of them rather than on the recipe.
func recipeTabs(panes []claudePane, v *view, inflate bool) []tab {
	tabs := make([]tab, len(v.groups))
	for i, g := range v.groups {
		sel := i == v.selected
		switch {
		case sel && inflate:
			var parts []string
			for _, m := range memberTabs(panes, g) {
				if m.selected {
					parts = append(parts, ansiReverse+m.text+ansiReset+ansiOrange)
				} else {
					parts = append(parts, m.text)
				}
			}
			tabs[i] = tab{text: fmt.Sprintf(" %s {%s} ", g.recipe, strings.Join(parts, "|"))}
		case g.hasRingers(panes):
			tabs[i] = tab{text: fmt.Sprintf(" %s %d %c ", g.recipe, len(g.members), g.worstState(panes)), selected: sel}
		default:
			tabs[i] = tab{text: fmt.Sprintf(" %s %c ", g.recipe, panes[g.members[0]].state), selected: sel}
		}
	}
	return tabs
}

// memberTabs builds a recipe's session row: the recipe's own session keeps
// its full "recipe:win" label, ringers shorten to "@name:win".
func memberTabs(panes []claudePane, g group) []tab {
	tabs := make([]tab, len(g.members))
	for j, i := range g.members {
		p := panes[i]
		label := p.label
		if _, name, ok := ringers.SplitSession(p.pane.SessionName); ok {
			label = fmt.Sprintf("%s%s:%d", ringers.Sep, name, p.pane.WindowIndex)
		}
		tabs[j] = tab{text: fmt.Sprintf(" %s %c ", label, p.state), selected: j == g.cursor}
	}
	return tabs
}

// layoutTabs splits the tabs into rows that fit in width columns, with a "|"
// between neighbours. prefix leads the first row (and counts toward it).
// A tab wider than the whole row still gets a row of its own; render
// truncates it.
func layoutTabs(tabs []tab, width int, prefix string) [][]tab {
	var rows [][]tab
	var row []tab
	used := utf8.RuneCountInString(prefix)
	for _, t := range tabs {
		w := visibleWidth(t.text)
		need := w
		if len(row) > 0 {
			need++ // the "|" separator
		}
		if len(row) > 0 && used+need > width {
			rows = append(rows, row)
			row, used, need = nil, 0, w
		}
		row = append(row, t)
		used += need
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	if len(rows) > 0 && prefix != "" {
		rows[0][0].text = prefix + rows[0][0].text
	}
	return rows
}

// visibleWidth counts a string's columns, skipping ANSI escape sequences.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}

// stripANSI removes \033[...m sequences.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '\033' && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = min(j+1, len(s))
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// truncVisible truncates a string to n visible columns, skipping ANSI
// escape sequences so they don't count toward the width.
func truncVisible(s string, n int) string {
	visible := 0
	i := 0
	for i < len(s) {
		// Skip ANSI escape sequences (\033[...m).
		if i+1 < len(s) && s[i] == '\033' && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++ // skip the 'm'
			}
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		visible++
		if visible > n {
			return s[:i]
		}
		i += size
	}
	return s
}

// --- Claude pane detection ---

type proc struct {
	ppid int
	args string
}

func findClaudePanes() ([]tmux.Pane, error) {
	all, err := tmux.ListAllPanes()
	if err != nil {
		return nil, fmt.Errorf("listing panes: %w", err)
	}

	procs, err := allProcesses()
	if err != nil {
		// Fallback: just check the pane's foreground command.
		var matched []tmux.Pane
		for _, p := range all {
			if strings.Contains(strings.ToLower(p.Command), "claude") {
				matched = append(matched, p)
			}
		}
		return matched, nil
	}

	// Build children map for tree walking.
	children := make(map[int][]int)
	for pid, p := range procs {
		children[p.ppid] = append(children[p.ppid], pid)
	}

	var matched []tmux.Pane
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Command), "claude") {
			matched = append(matched, p)
			continue
		}
		if hasClaude(p.PanePID, procs, children) {
			matched = append(matched, p)
		}
	}
	return matched, nil
}

func allProcesses() (map[int]proc, error) {
	out, err := exec.Command("ps", "-Ao", "pid=", "-o", "ppid=", "-o", "args=").Output()
	if err != nil {
		return nil, err
	}

	procs := make(map[int]proc)
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		if pid == 0 {
			continue
		}
		procs[pid] = proc{ppid: ppid, args: strings.Join(fields[2:], " ")}
	}
	return procs, nil
}

// hasClaude walks the process tree from rootPID looking for a claude process.
func hasClaude(rootPID int, procs map[int]proc, children map[int][]int) bool {
	queue := children[rootPID]
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if p, ok := procs[pid]; ok {
			if strings.Contains(strings.ToLower(p.args), "claude") {
				return true
			}
			queue = append(queue, children[pid]...)
		}
	}
	return false
}

// --- Text formatting ---

// lastLines trims trailing blank lines and returns the last n lines.
func lastLines(content string, n int) []string {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
