package icl

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/popup"
	"github.com/Josef-Hlink/twin/internal/ringers"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

// refreshEvery is how often the viewer recaptures every pane.
const refreshEvery = time.Second

// busyHold keeps a pane marked busy for a while after its last busy capture,
// so a spinner line that's briefly gone between two captures doesn't drop
// it to idle.
const busyHold = 3 * time.Second

// claudePane wraps a tmux pane with cached content and detected state.
type claudePane struct {
	pane    tmux.Pane
	label   string    // "session:window"
	content string    // captured pane output
	state   byte      // '*' busy, '&' menu, '-' idle
	busyAt  time.Time // when a capture last showed it busy
}

// capture (re)reads the pane's screen and the state it shows.
func (c *claudePane) capture(now time.Time) {
	content, _ := tmux.CapturePane(c.pane.Target)
	c.update(content, now)
}

// update takes in a new capture: a busy screen is remembered, and an idle
// one within busyHold of it still counts as busy. A menu always shows, as
// it's waiting on you.
func (c *claudePane) update(content string, now time.Time) {
	c.content = content
	c.state = detectState(stripANSI(content))
	switch {
	case c.state == '*':
		c.busyAt = now
	case c.state == '-' && now.Sub(c.busyAt) < busyHold:
		c.state = '*'
	}
}

const Usage = `usage: twin icl

interactive quickview of running claude agent panes: a sidebar lists them
all, each recipe's ringers nested under it, next to a preview of the
selected one, every pane recaptured each second.
  j / down      next session
  k / up        previous session
  J             next recipe
  K             previous recipe
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

// RunView renders the interactive Claude pane viewer. Runs inside the popup.
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
		cPanes[i] = claudePane{pane: p, label: label}
		cPanes[i].capture(time.Now())
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

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	v := newView(cPanes, cfg.Ringers.Color)

	// Reading stdin blocks, so it gets its own goroutine that hands each
	// read over a channel; the loop below then waits on keys and the
	// refresh ticker at once with select.
	keys := make(chan []byte)
	go func() {
		for {
			buf := make([]byte, 16)
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			keys <- buf[:n]
		}
	}()
	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()

	os.Stdout.WriteString(ansiHideCursor)
	defer os.Stdout.WriteString(ansiShowCursor)

	for {
		render(os.Stdout, cPanes, v, width, height)

		var buf []byte
		select {
		case now := <-ticker.C:
			for i := range cPanes {
				cPanes[i].capture(now)
			}
			continue
		case b, ok := <-keys:
			if !ok {
				return nil
			}
			buf = b
		}
		n := len(buf)

		// One read may hold several keys (pasted or sent at once), so walk
		// them all. Up/down arrive as ESC [ A/B and map onto k/j.
		for i := 0; i < n; i++ {
			key := buf[i]
			if key == 27 {
				if i+2 >= n || buf[i+1] != '[' {
					return nil // a bare Escape closes
				}
				key = map[byte]byte{'A': 'k', 'B': 'j'}[buf[i+2]]
				i += 2
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
			case 'j':
				v.move(1)
			case 'k':
				v.move(-1)
			case 'J':
				v.moveGroup(1)
			case 'K':
				v.moveGroup(-1)
			}
		}
	}
}

// --- Rows and selection ---

// row is one line of the sidebar: a Claude pane, or (pane -1) a header over
// a session's windows or a recipe's ringers.
type row struct {
	pane  int    // index into the pane list, -1 for a header
	label string // what the sidebar shows, tree glyph included
	group int    // which recipe the row belongs to, for J/K
	color string // SGR foreground of a ringer's rows ("34"), "" for the rest
}

// view is what the viewer navigates: every row at once, one of them selected.
type view struct {
	rows     []row
	selected int // index into rows, never a header
	top      int // first row shown once the list outgrows the popup
}

func newView(panes []claudePane, colorOf func(name string) config.Color) *view {
	v := &view{rows: buildRows(panes, colorOf)}
	if v.rows[0].pane < 0 {
		v.move(1)
	}
	return v
}

// node is one entry of the sidebar tree before it's flattened into rows.
type node struct {
	pane     int // -1 for a header
	label    string
	color    string // SGR, carried onto the node's row
	children []node
}

// buildRows lays the panes out as a tree, one recipe after another in the
// order they first appear. A recipe's own session sits at the root, its
// ringers nest under it, and a session with several Claude windows becomes
// a header over its windows:
//
//	klym            dots:1
//	├ :1            └ @blue
//	├ :2              ├ :1
//	└ @red:1          └ :3
//
// A ringer's rows are drawn in its [ringers.colors] color, from colorOf.
func buildRows(panes []claudePane, colorOf func(name string) config.Color) []row {
	var recipes []string
	sessions := make(map[string][]string) // recipe -> its sessions, in order
	windows := make(map[string][]int)     // session -> its panes, in order
	for i, p := range panes {
		s := p.pane.SessionName
		recipe := ringers.RecipeOf(s)
		if _, seen := sessions[recipe]; !seen {
			recipes = append(recipes, recipe)
		}
		if _, seen := windows[s]; !seen {
			sessions[recipe] = append(sessions[recipe], s)
		}
		windows[s] = append(windows[s], i)
	}

	// winLeaves turns a session's panes into ":win" leaves.
	winLeaves := func(s, color string) []node {
		var leaves []node
		for _, i := range windows[s] {
			label := fmt.Sprintf(":%d", panes[i].pane.WindowIndex)
			leaves = append(leaves, node{pane: i, label: label, color: color})
		}
		return leaves
	}

	var rows []row
	for g, recipe := range recipes {
		root := node{pane: -1, label: recipe}
		var rung []node
		for _, s := range sessions[recipe] {
			_, name, isRinger := ringers.SplitSession(s)
			if !isRinger {
				root.children = winLeaves(s, "")
				continue
			}
			color := colorOf(name).ANSI()
			ringer := node{pane: -1, label: ringers.Sep + name, color: color, children: winLeaves(s, color)}
			rung = append(rung, ringer)
		}
		root.children = append(root.children, rung...)
		rows = flatten(rows, collapse(root), "", "", g)
	}
	return rows
}

// collapse folds a header over a single window into that window's row
// ("dots" over ":1" becomes "dots:1"), all the way down the tree. A recipe
// whose own session has one window keeps its ringers under that row.
func collapse(n node) node {
	for i, c := range n.children {
		n.children[i] = collapse(c)
	}
	if n.pane < 0 && len(n.children) > 0 && strings.HasPrefix(n.children[0].label, ":") {
		if len(n.children) == 1 || !strings.HasPrefix(n.children[1].label, ":") {
			leaf := n.children[0]
			n.pane, n.label = leaf.pane, n.label+leaf.label
			n.children = n.children[1:]
		}
	}
	return n
}

// flatten appends n and its subtree to rows, drawing the tree glyphs:
// glyph leads n's own row, indent leads every row below it.
func flatten(rows []row, n node, glyph, indent string, group int) []row {
	rows = append(rows, row{pane: n.pane, label: glyph + n.label, group: group, color: n.color})
	for i, c := range n.children {
		if i == len(n.children)-1 {
			rows = flatten(rows, c, indent+"└ ", indent+"  ", group)
		} else {
			rows = flatten(rows, c, indent+"├ ", indent+"│ ", group)
		}
	}
	return rows
}

func (v *view) current() int {
	return v.rows[v.selected].pane
}

// move steps d rows (+1 down, -1 up), wrapping around and skipping headers.
func (v *view) move(d int) {
	n := len(v.rows)
	for {
		v.selected = (v.selected + d + n) % n
		if v.rows[v.selected].pane >= 0 {
			return
		}
	}
}

// moveGroup jumps to the first session of the next (d=1) or previous (d=-1)
// recipe, wrapping around.
func (v *view) moveGroup(d int) {
	groups := v.rows[len(v.rows)-1].group + 1
	target := (v.rows[v.selected].group + d + groups) % groups
	for i, r := range v.rows {
		if r.group == target && r.pane >= 0 {
			v.selected = i
			return
		}
	}
}

// scroll keeps the selected row within the height rows on screen, along with
// the header right above it, if any.
func (v *view) scroll(height int) {
	first := v.selected
	if first > 0 && v.rows[first-1].pane < 0 {
		first--
	}
	if first < v.top {
		v.top = first
	}
	if v.selected >= v.top+height {
		v.top = v.selected - height + 1
	}
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

// ansiRe matches a CSI escape sequence, such as a color change.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// stripANSI drops escape sequences, which capture-pane -e keeps. Claude's
// spinner recolors its verb as it pulses, so "Crunching…" can come out with
// a color change glued on and no longer end in "ing…".
func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// detectState scans the last ~20 lines of plain (escape-free) content to
// determine pane state.
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
	ansiHome    = "\033[H" // cursor to the top left
	ansiEOL     = "\033[K" // erase to the end of the line
	ansiEOS     = "\033[J" // erase to the end of the screen
	ansiDim     = "\033[2m"

	ansiHideCursor = "\033[?25l"
	ansiShowCursor = "\033[?25h"
)

// render draws the full TUI: the session sidebar on the left, the selected
// pane's preview on the right, and a status line below both.
func render(w io.Writer, panes []claudePane, v *view, width, height int) {
	var b strings.Builder

	// Draw over the last frame rather than clearing the screen first, which
	// would flicker on every refresh: each line erases what's left of the
	// old one past its end.
	b.WriteString(ansiHome)

	bodyHeight := max(height-1, 1) // all but the status line
	v.scroll(bodyHeight)

	// The sidebar fits its longest label, up to half the popup.
	labelW := 0
	for _, r := range v.rows {
		labelW = max(labelW, utf8.RuneCountInString(r.label))
	}
	labelW = min(labelW, max(width/2-4, 1))
	previewW := max(width-labelW-5, 1) // " label s " + the "┃" separator

	preview := lastLines(panes[v.current()].content, bodyHeight)

	for i := range bodyHeight {
		b.WriteString(sidebarCell(panes, v, v.top+i, labelW))
		b.WriteString(ansiOrange + "┃" + ansiReset)
		if i < len(preview) {
			b.WriteString(truncVisible(preview[i], previewW))
			b.WriteString(ansiReset) // prevent color bleed across lines
		}
		b.WriteString(ansiEOL + "\r\n")
	}

	// --- Status line (last line) ---
	b.WriteString(ansiDim)
	status := " j/k: session  J/K: recipe  Enter: switch  q: close"
	b.WriteString(truncVisible(status, width))
	b.WriteString(ansiReset + ansiEOL + ansiEOS)

	w.Write([]byte(b.String()))
}

// sidebarCell renders row i as " label state ", labelW+4 columns wide: in
// orange, or a ringer's own color; the selected row reversed, a header
// dimmed, blank past the last row.
func sidebarCell(panes []claudePane, v *view, i, labelW int) string {
	if i >= len(v.rows) {
		return strings.Repeat(" ", labelW+4)
	}
	r := v.rows[i]
	label := truncVisible(r.label, labelW)
	label += strings.Repeat(" ", labelW-utf8.RuneCountInString(label))
	color := ansiOrange
	if r.color != "" {
		color = "\033[" + r.color + "m"
	}
	if r.pane < 0 {
		return color + ansiDim + " " + label + "   " + ansiReset
	}
	cell := fmt.Sprintf(" %s %c ", label, panes[r.pane].state)
	if i == v.selected {
		return color + ansiReverse + cell + ansiReset
	}
	return color + cell + ansiReset
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
