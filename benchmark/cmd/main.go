package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/pyroscope-io/pyroscope/benchmark/cmd/command"
)

func main() {
	if err := command.Initialize(); err != nil {
		fatalf("%s %v\n\n", color.RedString("Error:"), err)
	}
}

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}
