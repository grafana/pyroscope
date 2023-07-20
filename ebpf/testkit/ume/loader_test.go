package ume

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"regexp"
	"testing"
	"time"
	"unsafe"
)

func TestLoad(t *testing.T) {
	ume, err := New("/home/korniltsev/pyro/pyroscope/ebpf/bpf/pyperf.so", "on_event")
	require.NoError(t, err)

	//waitForDebugger()
	ume.invoke(unsafe.Pointer(uintptr(239)))

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
