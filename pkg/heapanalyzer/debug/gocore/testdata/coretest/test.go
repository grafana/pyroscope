package main

import (
	"fmt"
	"syscall"
)

func main() {
	inf := int64(syscall.RLIM_INFINITY)
	lim := syscall.Rlimit{
		Cur: uint64(inf),
		Max: uint64(inf),
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_CORE, &lim); err != nil {
		panic(fmt.Sprintf("error setting rlimit: %v", err))
	}

	_ = *(*int)(nil)
}
