package main

import (
	"os"

	"github.com/kloudyuk/tfi/cmd"

	"github.com/fatih/color"
)

var e = color.New(color.FgRed)

func main() {
	if err := cmd.Execute(); err != nil {
		e.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}
