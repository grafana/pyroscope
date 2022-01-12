package util

import (
	"path/filepath"
)

// Retrieve pyroscope data directory
func DataDirectory() string {
	return filepath.Join(dataBaseDirectory(), "pyroscope")
}
