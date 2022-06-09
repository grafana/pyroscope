package disk

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa364937(v=vs.85).aspx
	getDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func FreeSpace(path string) (bytesize.ByteSize, error) {
	dirPath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	var (
		freeBytesAvailableToCaller uint64
		totalNumberOfBytes         uint64
		totalNumberOfFreeBytes     uint64
	)

	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(dirPath)),
		uintptr(unsafe.Pointer(&freeBytesAvailableToCaller)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)))
	if ret == 0 {
		return 0, os.NewSyscallError("GetDiskFreeSpaceEx", err)
	}

	return bytesize.ByteSize(freeBytesAvailableToCaller), nil
}
