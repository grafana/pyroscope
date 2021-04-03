package gospy

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var example = []byte(`goroutine 19 [running]:
io.(*pipe).Read(0x140001481e0, 0x1400015e000, 0x1000, 0x1000, 0x1, 0x1000, 0x105d267d0)
	io/pipe.go:57 +0x8c
io.(*PipeReader).Read(0x14000146028, 0x1400015e000, 0x1000, 0x1000, 0x0, 0x0, 0x1400004ce58)
	io/pipe.go:134 +0x44
bufio.(*Reader).fill(0x14000148240)
	bufio/bufio.go:101 +0xf8
bufio.(*Reader).ReadByte(0x14000148240, 0x1000, 0x1000, 0x1400007e000)
	bufio/bufio.go:253 +0x34
github.com/aybabtme/rgbterm.interpret(0x105342a28, 0x14000148240, 0x105343548, 0x14000146008, 0x1053340d8, 0x0, 0x0)
	github.com/aybabtme/rgbterm@v0.0.0-20170906152045-cc83f3b3ce59/interpret.go:35 +0xac
github.com/aybabtme/rgbterm.Interpret(0x105342a28, 0x14000148240, 0x105343548, 0x14000146008, 0x0, 0x14000156040)
	github.com/aybabtme/rgbterm@v0.0.0-20170906152045-cc83f3b3ce59/interpret.go:152 +0x4c
created by github.com/aybabtme/rgbterm.NewInterpretingWriter
	github.com/aybabtme/rgbterm@v0.0.0-20170906152045-cc83f3b3ce59/interpret.go:180 +0x1fc

goroutine 20 [running]:
io.(*pipe).Read(0x140001482a0, 0x1400015f000, 0x1000, 0x1000, 0x1, 0x1000, 0x12cb65328)
	io/pipe.go:57 +0x8c
io.(*PipeReader).Read(0x14000146038, 0x1400015f000, 0x1000, 0x1000, 0x0, 0x0, 0x1400004d658)
	io/pipe.go:134 +0x44
bufio.(*Reader).fill(0x14000148300)
	bufio/bufio.go:101 +0xf8
bufio.(*Reader).ReadByte(0x14000148300, 0x1000, 0x1000, 0x14000180000)
	bufio/bufio.go:253 +0x34
github.com/aybabtme/rgbterm.interpret(0x105342a28, 0x14000148300, 0x105343548, 0x14000146010, 0x1053340d8, 0x0, 0x0)
	github.com/aybabtme/rgbterm@v0.0.0-20170906152045-cc83f3b3ce59/interpret.go:35 +0xac
github.com/aybabtme/rgbterm.Interpret(0x105342a28, 0x14000148300, 0x105343548, 0x14000146010, 0x0, 0x14000156040)
	github.com/aybabtme/rgbterm@v0.0.0-20170906152045-cc83f3b3ce59/interpret.go:152 +0x4c
created by github.com/aybabtme/rgbterm.NewInterpretingWriter
	github.com/aybabtme/rgbterm@v0.0.0-20170906152045-cc83f3b3ce59/interpret.go:180 +0x1fc

`)

var _ = Describe("gospy", func() {
	Describe("Parse", func() {
		FIt("works as expected", func(done Done) {
			buf := bytes.NewBuffer(example)
			names := []string{}
			Parse(buf, func(name []byte, samples uint64, err error) {
				names = append(names, string(name))
			})
			Expect(names).To(ConsistOf("io.(*pipe).Read:57"))
		})
	})
})
