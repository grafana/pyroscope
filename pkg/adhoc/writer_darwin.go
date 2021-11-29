package adhoc

import (
	"os"
	"path/filepath"
)

func dataBaseDirectory() string {
	homeDir, ok := os.LookupEnv("HOME")
	if !ok {
		homeDir = "/"
	}
	return filepath.Join(homeDir, "Library", "Application Support")
}
