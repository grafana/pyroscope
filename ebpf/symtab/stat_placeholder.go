//go:build !linux && !darwin

package symtab

import (
	"os"
)

type Stat struct {
	Dev uint64
	Ino uint64
}

func statFromFileInfo(file os.FileInfo) Stat {
	return Stat{}
}
