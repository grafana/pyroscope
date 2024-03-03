package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"unsafe"
	//"golang.org/x/sys/unix"
	//"golang.org/x/sys/unix"
)

//go:noinline
func dump() {
	println("====================")
	ff, err := os.Open("/dev/null")
	if err != nil {
		panic(err)
	}
	debug.WriteHeapDump(ff.Fd())
	fmt.Println("dumped")
}
func main() {
	root = new(Foo)
	root.name = "root"
	pp(root)
	loop()
}

var root *Foo

//go:noinline
func loop3() bool {
	return true
}

//go:noinline
func loop2() {
	cont := true
	for cont {
		cont = loop3()
	}
}

//go:noinline
func loop() {
	cnt := 0
	loop := 0
	var a *Foo
	var b *Foo
	for {
		if cnt > 0 {
			loop2()
		}

		if cnt == 0 {
			loop += 1
			a = new(Foo)
			a.name = "foo.a"
			b = new(Foo)
			b.name = "foo.b"
			a.Next = b
			b.Next = a
			pp(a)
			pp(b)
			dump()
		}
		cnt += 1
		if loop > 2 {
			break
		}
	}
	fmt.Printf("+%+v %+v", a, b)
}

func pp(a *Foo) {
	println(strconv.FormatInt(int64(uintptr(unsafe.Pointer(a))), 16))
}

type Foo struct {
	Next *Foo
	name string
}
