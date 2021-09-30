package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/pyroscope-io/pyroscope/benchmark/cmd/command"
)

func main() {
	if err := command.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n\n", color.RedString("Error:"), err)
		os.Exit(1)
	}
}
