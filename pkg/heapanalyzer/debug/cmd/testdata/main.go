package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"time"
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

type StructWithPointers struct {
	s  string
	is []int
	p  *int
	f  *Foo
	fs []*Foo
}

var globfs []*Foo

//go:noinline
func loop() {
	var arr [0x30]byte
	cnt := 0
	loop := 0
	var a *Foo
	var b *Foo
	var ss StructWithPointers
	var ssp *StructWithPointers = new(StructWithPointers)
	var foos []*Foo
	var foosArray [2]*Foo
	var foosArrayNoPointers [2]Foo
	var iiints []byte
	for {
		if cnt > 0 {
			loop2()
		}

		if cnt == 0 {
			ss.s = fmt.Sprintf("qwe %d", time.Now().UnixMilli())
			ss.p = new(int)
			*ss.p = 239
			ss.f = new(Foo)
			ss.f.name = "ss.f"
			ss.fs = make([]*Foo, 2)
			ss.fs[0] = new(Foo)
			ss.fs[0].name = "ss.fs[0]"
			ss.fs[1] = new(Foo)
			ss.fs[1].name = "ss.fs[1]"
			fmt.Printf("%+v\n", ss)
			loop += 1
			a = new(Foo)
			a.name = "foo.a"
			b = new(Foo)
			b.name = "foo.b"
			a.Next = b
			b.Next = a
			foos = append(foos, a)
			foos = append(foos, b)
			foos = append(foos, new(Foo))
			iiints = append(iiints, 1)
			iiints = append(iiints, 2)
			iiints = append(iiints, 3)
			foosArray[0] = a
			foosArray[1] = b
			foosArrayNoPointers[0] = *a
			foosArrayNoPointers[1] = *b
			*ssp = ss
			globfs = append(globfs, a)
			globfs = append(globfs, b)
			globfs = append(globfs, new(Foo))
			globfs = append(globfs, new(Foo))
			globfs = append(globfs, new(Foo))
			pp(a)
			pp(b)
			dump()
		}
		cnt += 1
		if loop > 2 {
			break
		}
	}
	fmt.Printf("+%+v %+v %+v %+v %+v %+v", a, b, foos, iiints, foosArray, foosArrayNoPointers)
	fmt.Printf("+%+v %+v %+v %+v %+v %+v", a, b, arr, ssp)
}

func pp(a *Foo) {
	println(strconv.FormatInt(int64(uintptr(unsafe.Pointer(a))), 16))
}

type Foo struct {
	Next *Foo
	name string
}
