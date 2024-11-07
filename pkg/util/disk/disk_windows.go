//go:build windows

package diskutil

import (
	"syscall"
	"unsafe"
)

type volumeChecker struct {
	minFreeDisk                uint64
	minDiskAvailablePercentage float64
}

func NewVolumeChecker(minFreeDisk uint64, minDiskAvailablePercentage float64) VolumeChecker {
	return &volumeChecker{
		minFreeDisk:                minFreeDisk,
		minDiskAvailablePercentage: minDiskAvailablePercentage,
	}
}

func (v *volumeChecker) HasHighDiskUtilization(path string) (*VolumeStats, error) {
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	r1, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r1 == 0 {
		return nil, err
	}

	stats := &VolumeStats{
		BytesAvailable: freeBytesAvailable,
	}

	percentageAvailable := float64(freeBytesAvailable) / float64(totalNumberOfBytes)

	if stats.BytesAvailable >= v.minFreeDisk {
		return stats, nil
	}

	if percentageAvailable > v.minDiskAvailablePercentage {
		return stats, nil
	}

	stats.HighDiskUtilization = true
	return stats, nil
}
