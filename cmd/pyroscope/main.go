package main

import (
	"github.com/fatih/color"
	"github.com/pyroscope-io/pyroscope/cmd/pyroscope/command"
)

func main() {
	if err := command.Initialize(); err != nil {
		fatalf("%s %v\n\n", color.RedString("Error:"), err)
	}
}
