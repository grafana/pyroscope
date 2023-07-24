package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"path/filepath"
	"unsafe"

	"github.com/cilium/ebpf"
	ebpfspy "github.com/grafana/phlare/ebpf"
	"github.com/grafana/phlare/ebpf/testkit/ume"
)

var pid = flag.Int("pid", -1, "")
var gdb = flag.Bool("gdb", false, "")

func main() {
	flag.Parse()
	pid := *pid

	proc, err := ume.NewProc(pid, pid)
	if err != nil {
		panic(err)
	}
	data, err := ebpfspy.GetPyPerfPidData(pid)
	if err != nil {
		panic(err)
	}
	f, _ := filepath.Abs("../../profile_ume_x86.so")
	e, err := ume.New(f, "on_event")
	if err != nil {
		panic(err)
	}

	e.SetMem(proc)
	e.SetPIDTGID(uint32(pid), uint32(pid))

	pidConfig := ume.NewHashMap[uint32, ebpfspy.ProfilePyPidData]()
	pySymbols := ume.NewHashMap[ebpfspy.ProfilePySymbol, uint32]()
	stateHeap := ume.NewArrayMap[ebpfspy.ProfilePySampleStateT](1)
	pyEvents := ume.NewPerfEventMap(239)
	e.SetMap("py_state_heap", stateHeap)
	e.SetMap("py_pid_config", pidConfig)
	e.SetMap("py_events", pyEvents)
	e.SetMap("py_symbols", pySymbols)

	pidConfig.Update(uint32(pid), *data, ebpf.UpdateAny)

	err = proc.Stop()
	if err != nil {
		panic(err)
	}
	regs, err := proc.PtraceGetRegs()
	if err != nil {
		panic(err)
	}
	fmt.Printf("%x\n", regs.Fs_base)
	currentTask := make([]byte, 0x2000)
	putFSBase(currentTask, regs.Fs_base)
	e.SetCurrentTask(currentTask)

	if *gdb {
		//ume.StartGDBServer()
		ume.WaitForDebugger()
	}
	e.Invoke(unsafe.Pointer(uintptr(239)))
}

func putFSBase(currentTask []byte, val uint64) {
	binary.LittleEndian.PutUint64(currentTask[0x1b68:], uint64(val))
}
