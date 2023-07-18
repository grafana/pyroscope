//go:build linux

package symtab

import (
	"os"
	"syscall"
)

type Stat struct {
	Dev   uint64
	Inode uint64
}

func statFromFileInfo(file os.FileInfo) Stat {
	sys := file.Sys()
	sysStat, ok := sys.(*syscall.Stat_t)
	if !ok || sysStat == nil {
		return Stat{}
	}
	return Stat{
		Dev:   sysStat.Dev,
		Inode: sysStat.Ino,
	}
}
