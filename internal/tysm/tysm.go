package tysm

import (
	"flag"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin tysm [--message <text> | -m <text>]

teardown: kill the tmux server and print a farewell message.
  --message, -m custom farewell message
`

const defaultMsg = "thank you so much twin 🥀"

// Run kills the tmux server and prints a farewell message.
// When running inside tmux, it spawns a detached process to print the message
// to the parent terminal's TTY after the server is gone.
func Run(args []string) error {
	fs := flag.NewFlagSet("tysm", flag.ContinueOnError)
	message := fs.String("message", "", "custom farewell message")
	fs.StringVar(message, "m", "", "custom farewell message (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	msg := resolveMessage(*message, cfg.TysmMsg)

	if !tmux.InTmux() {
		if err := tmux.KillServer(); err != nil {
			return err
		}
		fmt.Println(msg)
		return nil
	}

	// Inside tmux: grab the parent terminal's TTY and PID before detaching,
	// then hand off to a background process that prints the farewell cleanly.
	tty, err := tmux.ClientTTY()
	if err != nil {
		return fmt.Errorf("getting client tty: %w", err)
	}
	pid, err := tmux.ClientPID()
	if err != nil {
		return fmt.Errorf("getting client pid: %w", err)
	}

	// Detach first so the parent shell's "tmux attach" exits 0.
	if err := tmux.DetachClient(); err != nil {
		return fmt.Errorf("detaching client: %w", err)
	}

	// Spawn a detached process: brief sleep for the prompt to redraw,
	// print the farewell below it, kill the server, then SIGWINCH the
	// parent shell so it redraws its prompt underneath the message.
	// Setsid puts it in its own session group so it survives kill-server.
	cmd := exec.Command("sh", "-c", fmt.Sprintf(
		`sleep 0.2 && printf '\n%%s\n' %q > %q && tmux kill-server 2>/dev/null; kill -WINCH %s 2>/dev/null`,
		msg, tty, pid,
	))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawning farewell process: %w", err)
	}

	return nil
}

func resolveMessage(flag, cfg string) string {
	if flag != "" {
		return flag
	}
	if cfg != "" {
		return cfg
	}
	return defaultMsg
}
