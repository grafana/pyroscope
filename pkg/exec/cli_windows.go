package exec

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole         = kernel32.NewProc("AttachConsole")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

func performOSChecks(_ string) error { return nil }

func adjustCmd(_ *exec.Cmd, _ config.Exec) error { return nil }

func processExists(pid int) bool {
	const da = syscall.STANDARD_RIGHTS_READ |
		syscall.PROCESS_QUERY_INFORMATION |
		syscall.SYNCHRONIZE
	h, err := syscall.OpenProcess(da, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() {
		_ = windows.Close(windows.Handle(h))
	}()
	// Refer to Microsoft documentation:
	// https://docs.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getexitcodeprocess
	var exitCode uint32
	err = windows.GetExitCodeProcess(windows.Handle(h), &exitCode)
	if err != nil {
		return false
	}
	const STILL_ACTIVE = 259
	return exitCode == STILL_ACTIVE
}

func sendSignal(p *os.Process, s os.Signal) error {
	if p == nil || p.Pid == 0 {
		return nil
	}
	// On Windows Go runtime does not handle signals (events) other than SIGINT.
	if s == syscall.SIGINT {
		if err := sendCtrlC(p.Pid); err == nil {
			return nil
		}
	}
	return p.Kill()
}

// A process can be attached to at most one console.
// Refer to https://docs.microsoft.com/en-us/windows/console/attachconsole.
var console sync.Mutex

// Interrupt sends CTRL+C signal to the console.
// Refer to https://github.com/golang/go/issues/6720 for details.
func sendCtrlC(pid int) (err error) {
	console.Lock()
	defer console.Unlock()
	ret, _, err := procAttachConsole.Call(uintptr(pid))
	if ret == 0 && err != windows.ERROR_ACCESS_DENIED {
		// A process can be attached to at most one console. If the calling
		// process is already attached to a console, the error code returned is
		// ERROR_ACCESS_DENIED (5). If the specified process does not have a
		// console, the error code returned is ERROR_INVALID_HANDLE (6). If the
		// specified process does not exist, the error code returned is
		// ERROR_INVALID_PARAMETER (87).
		return fmt.Errorf("AttachConsole: %w", err)
	}
	// Disable events handling for the current process
	ret, _, err = procSetConsoleCtrlHandler.Call(0, 1)
	if ret == 0 {
		return fmt.Errorf("SetConsoleCtrlHandler: %w", err)
	}
	// Note on CTRL_C_EVENT: This signal cannot be generated for process
	// groups. If dwProcessGroupId is nonzero, this function will succeed, but
	// the CTRL+C signal will not be received by processes within the specified
	// process group.
	if err = windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0); err != nil {
		return fmt.Errorf("GenerateConsoleCtrlEvent: %w", err)
	}
	// A process can use the FreeConsole function to detach itself from its
	// console. If other processes share the console, the console is not
	// destroyed, but the process that called FreeConsole cannot refer to it.
	// If the calling process is not already attached to a console,
	// the error code returned is ERROR_INVALID_PARAMETER (87).
	// The console must be released immediately after GenerateConsoleCtrlEvent.
	_, _, _ = procFreeConsole.Call()
	return nil
}
