//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/hashicorp/go-multierror"
	"strings"
	"sync"

	"github.com/iovisor/gobpf/bcc"
)

// TODO: optimize this

func walkStack(stackTrace *bcc.Table, pid, rootId []byte) string {
	stack, _ := stackTrace.Get(rootId)

	var pidInt uint32
	r2 := bytes.NewReader(pid)
	binary.Read(r2, binary.LittleEndian, &pidInt)
	var stackFrames []string
	for len(stack) >= 8 && !bytes.Equal(stack[:8], []byte{0, 0, 0, 0, 0, 0, 0, 0}) {
		addr := stack[:8]
		stack = stack[8:]

		var addrInt uint64
		r := bytes.NewReader(addr)
		binary.Read(r, binary.LittleEndian, &addrInt)
		name := globalCache.sym(pidInt, addrInt)
		stackFrames = append(stackFrames, name+";")
	}
	reverse(stackFrames)
	return strings.Join(stackFrames, "")
}

func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

type session struct {
	pid        int
	sampleRate uint32
	modMutex   sync.Mutex
	mod        *bcc.Module
}

func newSession(pid int, sampleRate uint32) *session {
	return &session{pid: pid, sampleRate: sampleRate}
}

// this is a rough copy of what's going on in https://github.com/iovisor/bcc/blob/master/tools/profile.py
func (s *session) Start() error {
	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	s.mod = bcc.NewModule(s.createProgram(), []string{})
	if s.mod == nil {
		return errors.New("module error")
	}
	fd, err := s.mod.LoadPerfEvent("do_perf_event")

	if err != nil {
		return err
	}

	evType := 1   // -1 // PerfType.SOFTWARE
	evConfig := 0 // -1 // PerfSWConfig.CPU_CLOCK
	samplePeriod := 0
	sampleFreq := int(s.sampleRate)
	pid := -1
	cpu := -1
	groupFd := -1

	err = s.mod.AttachPerfEvent(evType, evConfig, samplePeriod, sampleFreq, pid, cpu, groupFd, fd)
	if err != nil {
		return err
	}

	return nil
}

func (s *session) createProgram() string {
	var threadFilter string
	if s.pid < 0 {
		threadFilter = "1"
	} else {
		threadFilter = fmt.Sprintf("tgid == %d", s.pid)
	}
	return strings.Replace(BPF_PROGRAM, "THREAD_FILTER", threadFilter, 1)
}

func (s *session) Reset(cb func([]byte, uint64) error) error {
	var errs error
	s.modMutex.Lock()
	defer s.modMutex.Unlock()

	countsId := s.mod.TableId("counts")
	stackTracesId := s.mod.TableId("stack_traces")

	ct := bcc.NewTable(countsId, s.mod)
	st := bcc.NewTable(stackTracesId, s.mod)

	iter := ct.Iter()
	for iter.Next() {
		k := UnpackKeyBytes(iter.Key())
		// TODO: optimize this
		line := string(k.name) + ";"
		line += walkStack(st, k.pid, k.user_stack_id)
		line += walkStack(st, []byte{0, 0, 0, 0}, k.kernel_stack_id)

		v := iter.Leaf()
		var valInt uint64
		buf := bytes.NewBuffer(v)
		binary.Read(buf, binary.LittleEndian, &valInt) // TODO: not sure if it's little endian
		lb := []byte(line)
		if len(lb) > 0 {
			err := cb(lb[:len(lb)-1], valInt)
			if err != nil {
				errs = multierror.Append(errs, errs)
			}
		}
	}

	ct.DeleteAll()
	st.DeleteAll()
	return errs

	// s.mod.Close()
	// return s.Start()
}
