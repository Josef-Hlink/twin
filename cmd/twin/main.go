package main

import (
	"fmt"
	"os"

	"github.com/Josef-Hlink/twin/internal/fr"
	"github.com/Josef-Hlink/twin/internal/icl"
	"github.com/Josef-Hlink/twin/internal/pbqt"
	"github.com/Josef-Hlink/twin/internal/shkspr"
	"github.com/Josef-Hlink/twin/internal/sybau"
	"github.com/Josef-Hlink/twin/internal/tspmo"
	"github.com/Josef-Hlink/twin/internal/tysm"
)

var helps = map[string]string{
	"tspmo":  tspmo.Usage,
	"fr":     fr.Usage,
	"sybau":  sybau.Usage,
	"pbqt":   pbqt.Usage,
	"icl":    icl.Usage,
	"shkspr": shkspr.Usage,
	"tysm":   tysm.Usage,
}

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	sub := os.Args[1]
	args := os.Args[2:]

	if sub == "-h" || sub == "--help" {
		printUsage(os.Stdout)
		return
	}
	if wantsHelp(args) {
		if u, ok := helps[sub]; ok {
			fmt.Print(u)
			return
		}
	}

	var err error

	switch sub {
	case "tspmo":
		err = tspmo.Run()
	case "fr":
		err = fr.Run(args)
	case "sybau":
		err = sybau.Run(args)
	case "fr-picker":
		err = fr.RunPicker()
	case "sybau-picker":
		err = sybau.RunPicker(args)
	case "pbqt":
		err = pbqt.Run(args)
	case "pbqt-picker":
		err = pbqt.RunPicker()
	case "icl":
		err = icl.Run()
	case "icl-view":
		err = icl.RunView()
	case "tysm":
		err = tysm.Run(args)
	case "shkspr":
		err = shkspr.Run(args)
	default:
		printUsage(os.Stderr)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "twin %s: %v\n", sub, err)
		os.Exit(1)
	}
}

func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func printUsage(w *os.File) {
	const usage = `usage: twin <command> [args]

commands:
  tspmo    spin up tmux sessions from recipes
  fr       open a single recipe (fzf picker / name / --list)
  sybau    fzf-based session switcher
  pbqt     kill a tmux session (fzf picker / name)
  icl      quick-glance at running Claude agent panes
  shkspr   open twin.toml in $EDITOR
  tysm     kill tmux server with a farewell

run 'twin <command> --help' for command-specific help.
`
	fmt.Fprint(w, usage)
}
