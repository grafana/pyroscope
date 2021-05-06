package disk

import (
	"syscall"
)

const (
	b  = 1
	kB = 1024 * b
	mB = 1024 * kB
	gB = 1024 * mB
)

type UsagesStats struct {
	All  uint64
	Used uint64
	Free uint64
}

func usage(path string) (*UsagesStats, error) {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return nil, err
	}

	stat := UsagesStats{}
	stat.All = fs.Blocks * uint64(fs.Bsize)
	stat.Free = fs.Bfree * uint64(fs.Bsize)
	stat.Used = stat.All - stat.Free
	return &stat, nil
}

func IsRunningOutOfSpace(storagePath string, threshold uint64) bool {
	stats, err := usage(storagePath)
	if err != nil {
		return false
	}

	if stats.Free < threshold {
		return true
	}
	return false
}

func ShouldShowOutOfSpaceWarning(storagePath string, threshold uint64) bool {
	stats, err := usage(storagePath)
	if err != nil {
		return false
	}

	if stats.Free < threshold {
		return true
	}
	return false
}
