package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/pyroscope-io/pyroscope/scripts/dbtool/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, color.RedString("Error:"), err)
		os.Exit(1)
	}
}
