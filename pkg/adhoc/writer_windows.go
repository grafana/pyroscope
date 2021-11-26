package adhoc

import (
	"os"
	"path/filepath"
)

func dataBaseDirectory() string {
	return filepath.Join(os.Getenv("USERPROFILE"), os.Getenv("LOCALAPPDATA"))
}
