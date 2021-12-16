package util

import "path/filepath"

func DataDirectory() string {
	return filepath.Join(dataBaseDirectory(), "pyroscope")
}
