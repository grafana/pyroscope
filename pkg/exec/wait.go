package exec

import (
	"os"
	goexec "os/exec"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/util/process"
)

// WaitForProcess until it finishes, or it timeouts if the timeout is set.
func WaitForProcess(logger *logrus.Logger, cmd *goexec.Cmd, c chan os.Signal, timeout time.Duration, forwardSignals bool) error {
	tout := time.NewTimer(timeout)
	if timeout == 0 {
		tout.Stop()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case s := <-c:
			if !forwardSignals {
				return nil
			}
			_ = process.SendSignal(cmd.Process, s)
		case <-ticker.C:
			if !process.Exists(cmd.Process.Pid) {
				logger.Debug("child process exited")
				if cmd == nil {
					return nil
				}
				return cmd.Wait()
			}
		case <-tout.C:
			logger.Debug("timeout waiting for process to finish")
			return nil
		}
	}
}
