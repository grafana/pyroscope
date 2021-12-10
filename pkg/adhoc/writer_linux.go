package adhoc

import (
	"os"
	"path/filepath"
)

func dataBaseDirectory() string {
	if dir, ok := os.LookupEnv("XDG_DATA_HOME"); ok {
		return dir
	}
	homeDir, ok := os.LookupEnv("HOME")
	if !ok {
		homeDir = "/"
	}
	return filepath.Join(homeDir, ".local", "share")
}
