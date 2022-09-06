package pprof

import (
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"reflect"
	"unsafe"
)

type StackFrameFormatter interface {
	format(x *tree.Profile, fn *tree.Function, line *tree.Line) []byte
}

func unsafeStrToSlice(s string) []byte {
	return (*[0x7fff0000]byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data))[:len(s):len(s)]
}

type UnsafeFunctionNameFormatter struct {
}

func (UnsafeFunctionNameFormatter) format(x *tree.Profile, fn *tree.Function, _ *tree.Line) []byte {
	return unsafeStrToSlice(x.StringTable[fn.Name])
}

type RbspyFormatter struct {
}

func (RbspyFormatter) format(x *tree.Profile, fn *tree.Function, line *tree.Line) []byte {
	return []byte(fmt.Sprintf("%s:%d - %s",
		x.StringTable[fn.Filename],
		line.Line,
		x.StringTable[fn.Name]))
}

func StackFrameFormatterForSpyName(spyName string) StackFrameFormatter {
	if spyName == "rbspy" {
		return RbspyFormatter{}
	}
	return UnsafeFunctionNameFormatter{}
}
