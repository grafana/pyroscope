//go:build !unix

package symtab

import (
	"os"
)

type stat struct {
	dev uint64
	ino uint64
}

func statFromFileInfo(file os.FileInfo) stat {
	return stat{}
}
