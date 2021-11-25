package adhoc

import (
	"os"
	"path/filepath"
)

func dataBaseDirectory() string {
	return filepath.Join(os.GetEnv("USERPROFILE"), os.GetEnv("LOCALAPPDATA"))
}
