package main

import (
	"fmt"
	"os"

	"github.com/Josef-Hlink/twin/internal/sybau"
	"github.com/Josef-Hlink/twin/internal/tspmo"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error

	switch os.Args[1] {
	case "tspmo":
		err = tspmo.Run()
	case "sybau":
		err = sybau.Run()
	case "sybau-picker":
		err = sybau.RunPicker()
	default:
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "twin %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

func printUsage() {
	const usage = `usage: twin <command>

commands:
  tspmo    spin up tmux sessions from recipes
  sybau    fzf-based session switcher
`
	fmt.Fprint(os.Stderr, usage)
}
