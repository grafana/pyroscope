package target

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// https://docs.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-queryservicestatusex
func getPID(serviceName string) (int, error) {
	m, err := mgr.Connect()
	if err != nil {
		return 0, err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	switch {
	case err == nil:
	case errors.Is(err, syscall.ERROR_NOT_FOUND):
		return 0, ErrNotFound
	default:
		return 0, err
	}
	defer s.Close()
	// A variable that receives the number of bytes needed to store the status
	// information, if the function fails with ERROR_INSUFFICIENT_BUFFER.
	// Not used.
	var needed uint32
	var t windows.SERVICE_STATUS_PROCESS
	err = windows.QueryServiceStatusEx(s.Handle,
		windows.SC_STATUS_PROCESS_INFO,
		(*byte)(unsafe.Pointer(&t)),
		uint32(unsafe.Sizeof(t)),
		&needed)
	if err != nil {
		return 0, err
	}
	if t.ProcessId == 0 {
		return 0, ErrNotRunning
	}
	return int(t.ProcessId), nil
}
