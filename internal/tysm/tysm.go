package tysm

import (
	"flag"

	"github.com/Josef-Hlink/twin/internal/config"
	"github.com/Josef-Hlink/twin/internal/tmux"
)

const Usage = `usage: twin tysm [--message <text> | -m <text>]

teardown: kill the tmux server and print a farewell message.
  --message, -m custom farewell message
`

// Run loads the config, and just kills the tmux server.
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

	if err := tmux.KillServer(); err != nil {
		return err
	}

	// Print flag message if set, otherwise config message if set, otherwise default message
	if *message != "" {
		println(*message)
	} else if cfg.TysmMsg != "" {
		println(cfg.TysmMsg)
	} else {
		println("thank you so much twin 🥀")
	}

	return nil
}
