// +build !windows

package main

import (
	"fmt"
	"os"
)

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
	// revive:disable-next-line:deep-exit different impls exit at different points
	os.Exit(1)
}
