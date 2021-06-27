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
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procFreeConsole   = kernel32.NewProc("FreeConsole")
)

func performOSChecks(_ string) error {
	return nil
}

func adjustCmd(cmd *exec.Cmd, _ config.Exec) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	return nil
}

func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}

func sendSignal(p *os.Process, s os.Signal) error {
	if p == nil || p.Pid == 0 {
		return nil
	}
	if s == syscall.SIGINT {
		return sendEvent(p.Pid, windows.CTRL_BREAK_EVENT)
	}
	return p.Kill()
}

// A process can be attached to at most one console.
// Refer to https://docs.microsoft.com/en-us/windows/console/attachconsole.
var console sync.Mutex

// Interrupt sends CTRL+BREAK signal to the command process group.
// Refer to https://github.com/golang/go/issues/6720 for details.
func sendEvent(pid int, e uint32) (err error) {
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
	defer func() {
		// A process can use the FreeConsole function to detach itself from its
		// console. If other processes share the console, the console is not
		// destroyed, but the process that called FreeConsole cannot refer to it.
		// If the calling process is not already attached to a console,
		// the error code returned is ERROR_INVALID_PARAMETER (87).
		_, _, _ = procFreeConsole.Call()
	}()
	// Note on CTRL_C_EVENT: This signal cannot be generated for process
	// groups. If dwProcessGroupId is nonzero, this function will succeed, but
	// the CTRL+C signal will not be received by processes within the specified
	// process group.
	return windows.GenerateConsoleCtrlEvent(e, uint32(pid))
}
