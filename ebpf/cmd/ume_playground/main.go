package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	ebpfspy "github.com/grafana/phlare/ebpf"
	"github.com/grafana/phlare/ebpf/testkit/ume"
)

var pid = flag.Int("pid", -1, "")
var gdb = flag.Bool("gdb", false, "")

var dbgOnce sync.Once

func main() {
	flag.Parse()
	pid := *pid

	proc, err := ume.NewProc(pid, pid)
	if err != nil {
		panic(err)
	}
	defer proc.Close()

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

	currentTask := make([]byte, 0x2000)
	e.SetCurrentTask(currentTask)

	cnt := 0
	for {
		cnt += 1
		if err = proc.Wait(); err != nil {
			panic(err)
		}

		regs, err := proc.PtraceGetRegs()
		if err != nil {
			panic(err)
		}

		putFSBase(currentTask, regs.Fs_base)

		dbgOnce.Do(func() {
			if *gdb {
				ume.WaitForDebugger()
			}
		})
		e.Invoke(unsafe.Pointer(uintptr(239)))

		if err = proc.Continue(); err != nil {
			panic(err)
		}

		//if cnt%10 == 0 {
		printStacks(pySymbols, pyEvents)
		//}
		time.Sleep(1000 * time.Millisecond)
		if err = proc.Stop(); err != nil {
			panic(err)
		}
	}

}

func printStacks(pySymbols *ume.HashMap[ebpfspy.ProfilePySymbol, uint32], pyEvents *ume.PerfEventMap) {
	reverseSymbols := getSymbols(pySymbols)

	for {
		select {
		case e := <-pyEvents.Events():
			printStack(e, reverseSymbols)
			break
		default:
			fmt.Println("no more stacks")
			return
		}

	}
}

func getSymbols(pySymbols *ume.HashMap[ebpfspy.ProfilePySymbol, uint32]) map[uint32]ebpfspy.ProfilePySymbol {
	reverseSymbols := make(map[uint32]ebpfspy.ProfilePySymbol)
	for _, e := range pySymbols.Entries() {
		reverseSymbols[*e.V] = e.K
	}
	return reverseSymbols
}

func printStack(e []byte, reverseSymbols map[uint32]ebpfspy.ProfilePySymbol) {
	event := &ebpfspy.ProfilePyEvent{}
	if err := binary.Read(bytes.NewBuffer(e), binary.LittleEndian, event); err != nil {
		panic(err)
	}
	fmt.Println("==============")
	for i, symID := range event.Stack {
		if i >= int(event.StackLen) {
			break
		}
		//fmt.Printf("sym %8d\n", symID)
		symbol := reverseSymbols[symID]
		fmt.Printf("%10d %s | %s | %s \n", symID, strFromInt8(symbol.Name[:]), strFromInt8(symbol.Classname[:]), strFromInt8(symbol.File[:]))
	}
}

func putFSBase(currentTask []byte, val uint64) {
	binary.LittleEndian.PutUint64(currentTask[0x1b68:], uint64(val))
}

func strFromInt8(file []int8) any {
	u8 := make([]uint8, 0, len(file))
	for _, v := range file {
		if v == 0 {
			break
		}
		u8 = append(u8, uint8(v))
	}
	return string(u8)
}
