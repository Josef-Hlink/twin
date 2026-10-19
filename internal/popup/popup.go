package popup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const (
	chromeWidth       = 5 // fzf prompt + padding + tmux popup border (left/right)
	chromeHeight      = 4 // fzf prompt + status + tmux popup border (top/bottom)
	previewExtraWidth = 3 // preview border-left + padding
)

// Dims computes popup width and height from content metrics.
// When preview is false, the preview columns are ignored and the popup
// is sized for the list alone. maxItems, when > 0, caps the number of list
// rows the popup is sized for; fzf scrolls through any items beyond it.
func Dims(itemCount, maxItemLine, maxItems, maxPreviewLine, maxPreviewCount int, preview bool) (width, height int) {
	if maxItems > 0 && itemCount > maxItems {
		itemCount = maxItems
	}
	width = maxItemLine + chromeWidth
	height = itemCount + chromeHeight
	if preview {
		width += maxPreviewLine + previewExtraWidth
		height = max(itemCount, min(maxPreviewCount, 10)) + chromeHeight
	}
	return width, height
}

// Launch opens a tmux popup at the top-left running the given subcommand,
// with the border styled in `color`.
func Launch(title string, width, height int, subcommand string, color config.Color) error {
	return launch(tmux.PopupTopLeft, title, width, height, subcommand, color)
}

// LaunchCenter opens a tmux popup centered on screen.
func LaunchCenter(title string, width, height int, subcommand string, color config.Color) error {
	return launch(tmux.PopupCenter, title, width, height, subcommand, color)
}

func launch(anchor tmux.PopupAnchor, title string, width, height int, subcommand string, color config.Color) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	style := fmt.Sprintf("fg=%s bold", color.Tmux())
	return tmux.DisplayPopup(anchor, title, width, height, style, self+" "+subcommand)
}

// FzfSelect pipes items to fzf and returns the selected line.
// When previewCols > 0 and previewCmd is non-empty, a preview pane is shown.
// When accent is non-empty, fzf's pointer + marker are colored to match.
func FzfSelect(items []string, previewCols int, previewCmd string, accent config.Color) (string, error) {
	var fzfArgs []string
	if accent != "" {
		c := accent.Fzf()
		fzfArgs = append(fzfArgs, "--color", fmt.Sprintf("pointer:%s,marker:%s", c, c))
	}
	if previewCols > 0 && previewCmd != "" {
		fzfArgs = append(fzfArgs,
			"--preview", previewCmd,
			"--preview-window", fmt.Sprintf("right:%d:wrap:border-left", previewCols),
		)
	}

	return runFzf(items, fzfArgs)
}

// FzfPickOrType is FzfSelect for pickers that also accept new values: enter
// picks the highlighted item, or takes the typed text when nothing matches,
// and alt-enter always takes the typed text (fzf matches fuzzily, so a new
// name usually still highlights some existing item). Returns "" when
// cancelled.
func FzfPickOrType(items []string, prompt string, accent config.Color) (string, error) {
	fzfArgs := []string{
		"--prompt", prompt,
		"--header", "enter: pick · alt-enter: use typed text",
		"--bind", "enter:accept-or-print-query,alt-enter:print-query",
	}
	if accent != "" {
		c := accent.Fzf()
		fzfArgs = append(fzfArgs, "--color", fmt.Sprintf("pointer:%s,marker:%s", c, c))
	}
	return runFzf(items, fzfArgs)
}

// runFzf pipes items to fzf and returns what it printed, or "" when the user
// cancelled.
func runFzf(items []string, fzfArgs []string) (string, error) {
	cmd := exec.Command("fzf", fzfArgs...)
	cmd.Stdin = strings.NewReader(strings.Join(items, "\n"))
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		// fzf exits 130 on Escape, 1 on no match — not errors.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 130 || exitErr.ExitCode() == 1 {
				return "", nil
			}
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// Quote wraps s in single quotes for the shell that runs a popup's command,
// so arguments with spaces or special characters arrive intact.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Hold shows err inside a popup and waits for enter before letting it close,
// since a popup disappears (taking its output with it) as soon as its command
// exits. Returns nil once the error has been shown.
func Hold(cmd string, err error) error {
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "\ntwin %s: %v\n\npress enter to close", cmd, err)
	bufio.NewReader(os.Stdin).ReadString('\n')
	return nil
}
