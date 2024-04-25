package ebpfspy

import (
	"bufio"
	"fmt"
	"github.com/go-kit/log/level"
	"os"
	"syscall"
)

func (s *session) printBpfLog() {

	var (
		f   *os.File
		err error
	)
	fs := []string{
		"/sys/kernel/debug/tracing/trace_pipe",
		"/sys/kernel/tracing/trace_pipe",
	}
	for _, it := range fs {
		f, err = os.Open(it)
		if err == nil {
			break
		}
		level.Error(s.logger).Log("msg", "error opening trace_pipe", "err", err, "f", it)
	}
	hint := func() {
		level.Debug(s.logger).Log("msg", "trace_pipe not found. BPF log will not be printed.")
		level.Debug(s.logger).Log("msg", "try running # mount -t debugfs nodev /sys/kernel/debug &&  mount -t tracefs nodev /sys/kernel/debug/tracing")
	}
	if f == nil {
		const mountPath = "/pyroscope-tracefs"

		_ = os.Mkdir(mountPath, 0700)
		stat, _ := os.Stat(mountPath + "/trace_pipe")
		if stat == nil {
			level.Warn(s.logger).Log("msg", "trying to mount tracefs", "at", mountPath)
			err = syscall.Mount("tracefs", mountPath, "tracefs", 0, "")
			if err != nil {
				level.Error(s.logger).Log("msg", "error mounting tracefs", "err", err)
				hint()
				return
			}
			defer func() {
				err = syscall.Unmount(mountPath, 0)
				if err != nil {
					level.Error(s.logger).Log("msg", "error unmounting tracefs", "err", err)
				}
			}()
		}
		f, err = os.Open(mountPath + "/trace_pipe")
		if err != nil {
			level.Error(s.logger).Log("msg", "error opening trace_pipe", "err", err)
			hint()
			return
		}
	}
	level.Debug(s.logger).Log("msg", "printing BPF log", "from", f.Name())
	s.mutex.Lock()
	if !s.started {
		s.mutex.Unlock()
		return
	}
	s.bpflogFile = f
	s.mutex.Unlock()
	defer f.Close() //todo there is a race here racing with stop
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
}
