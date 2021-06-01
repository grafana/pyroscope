// +build !windows

package main

import (
	"fmt"
	"os"
)

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}
