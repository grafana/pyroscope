package ebpfspy

import (
	"fmt"
	"github.com/cilium/ebpf"
	ume "github.com/grafana/phlare/ebpf/testkit/ume"
	"github.com/stretchr/testify/require"
	"os"
	"regexp"
	"testing"
	"time"
	"unsafe"
)

func TestLoad(t *testing.T) {
	pid := uint32(4242)
	e, err := ume.New("/home/korniltsev/pyro/pyroscope/ebpf/bpf/pyperf.so", "on_event")
	require.NoError(t, err)

	e.SetPIDTGID(0xdead, pid)

	pidConfig := ume.NewHashMap[uint32, pyperfPidData]()
	e.SetMap("py_state_heap", ume.NewArrayMap[pyperfSampleStateT](1))
	e.SetMap("py_pid_config", pidConfig)

	pidConfig.Update(pid, pyperfPidData{}, ebpf.UpdateAny)

	//waitForDebugger()
	e.Invoke(unsafe.Pointer(uintptr(239)))

}

func waitForDebugger() {
	re := regexp.MustCompile("TracerPid:\\s+(\\d+)")
	for {
		fmt.Println("waiting for debugger to attach")
		time.Sleep(time.Second)
		status, err := os.ReadFile("/proc/self/status")
		if err != nil {
			fmt.Println(err)
		}
		submatches := re.FindAllStringSubmatch(string(status), -1)
		if len(submatches) != 1 {
			fmt.Println(" %w")
			continue
		}
		pid := submatches[0][1]
		if pid == "0" {
			continue
		}
		fmt.Printf("debugger attach %s\n", pid)
		break
	}

}

//func TestShim5(t *testing.T) {
//	f := func5(func(a, b, c, d, e uintptr) uintptr {
//		return 0xcafebabe
//	})
//	s := newFunc5Shim(&f)
//	_ = s
//	fmt.Printf("shim start %x\n", s.start)
//	res := fooGo(s.start)
//	fmt.Printf("fooRes %x\n", res)
//}
