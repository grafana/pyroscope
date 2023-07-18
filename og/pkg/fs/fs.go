// Package fs contain file system helpers
package fs

import (
	"fmt"
	"os"
)

func EnsureDirExists(dir string) error {
	if dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, os.ModeDir|os.ModePerm); err != nil {
		return fmt.Errorf("could not create directory '%s': %w", dir, err)
	}

	return nil
}
