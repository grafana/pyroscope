package util

import (
	"os"
	"path/filepath"
)

// Retrieve pyroscope data directory, creating it if needed.
func EnsureDataDirectory() (string, error) {
	dir := filepath.Join(dataBaseDirectory(), "pyroscope")
	return dir, os.MkdirAll(dir, os.ModeDir|os.ModePerm)
}
