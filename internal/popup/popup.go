package popup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Josef-Hlink/twin/internal/tmux"
)

const (
	chromeWidth       = 5 // fzf prompt + padding + tmux popup border (left/right)
	chromeHeight      = 4 // fzf prompt + status + tmux popup border (top/bottom)
	previewExtraWidth = 3 // preview border-left + padding
	borderStyle       = "fg=magenta bold"
)

// Dims computes popup width and height from content metrics.
// When preview is false, the preview columns are ignored and the popup
// is sized for the list alone.
func Dims(itemCount, maxItemLine, maxPreviewLine, maxPreviewCount int, preview bool) (width, height int) {
	width = maxItemLine + chromeWidth
	height = itemCount + chromeHeight
	if preview {
		width += maxPreviewLine + previewExtraWidth
		height = max(itemCount, min(maxPreviewCount, 10)) + chromeHeight
	}
	return width, height
}

// Launch opens a tmux popup running the given subcommand.
// It resolves the executable path automatically since the popup shell
// doesn't inherit PATH context.
func Launch(title string, width, height int, subcommand string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	return tmux.DisplayPopup(title, width, height, borderStyle, self+" "+subcommand)
}

// FzfSelect pipes items to fzf and returns the selected line.
// When previewCols > 0 and previewCmd is non-empty, a preview pane is shown.
func FzfSelect(items []string, previewCols int, previewCmd string) (string, error) {
	var fzfArgs []string
	if previewCols > 0 && previewCmd != "" {
		fzfArgs = append(fzfArgs,
			"--preview", previewCmd,
			"--preview-window", fmt.Sprintf("right:%d:wrap:border-left", previewCols),
		)
	}

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
