//go:build !windows
// +build !windows

package disk

import (
	"syscall"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

func Usage(path string) (UsageStats, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return UsageStats{}, err
	}
	u := UsageStats{
		Total:     bytesize.ByteSize(fs.Blocks) * bytesize.ByteSize(fs.Bsize),
		Available: bytesize.ByteSize(fs.Bavail) * bytesize.ByteSize(fs.Bsize),
	}
	return u, nil
}
