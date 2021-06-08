// +build !windows

package disk

import (
	"syscall"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

func FreeSpace(storagePath string) (bytesize.ByteSize, error) {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(storagePath, &fs)
	if err != nil {
		return 0, err
	}

	return bytesize.ByteSize(fs.Bfree) * bytesize.ByteSize(fs.Bsize), nil
}
