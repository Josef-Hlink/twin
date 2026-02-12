package main

import (
	"fmt"
	"os"

	"github.com/jdham/twin/internal/sybau"
	"github.com/jdham/twin/internal/tspmo"
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
	fmt.Fprintln(os.Stderr, "usage: twin <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  tspmo    spin up tmux sessions from recipes")
	fmt.Fprintln(os.Stderr, "  sybau    fzf-based session switcher")
}
