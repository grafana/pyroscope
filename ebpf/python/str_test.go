package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPythonString(t *testing.T) {
	testdata := []struct {
		buf []int8
		typ *PerfPyStrType
		res string
	}{
		{
			buf: []int8{-61},
			typ: &PerfPyStrType{Type: uint8(PyStrType1Byte), SizeCodepoints: 1},
			res: "Ã",
		},
		{
			buf: []int8{0x20, 0xa, 0x61},
			typ: &PerfPyStrType{Type: uint8(PyStrType1Byte | PyStrTypeAscii), SizeCodepoints: 2},
			res: " \n",
		},
		{
			buf: []int8{97, 0, 115, 0, 100, 0, 57, 4, 70, 4, 67, 4},
			typ: &PerfPyStrType{Type: uint8(PyStrType2Byte), SizeCodepoints: 6},
			res: "asdйцу",
		}, {
			buf: []int8{-61, 0, 0, 0, 35, -7, 1, 0},
			typ: &PerfPyStrType{Type: uint8(PyStrType4Byte), SizeCodepoints: 2},
			res: "Ã🤣",
		}, {
			buf: []int8{-61, -125, -48, -71, -47, -122, -47, -125},
			typ: &PerfPyStrType{Type: uint8(PyStrTypeUtf8), SizeCodepoints: 8},
			res: "Ãйцу",
		}, {
			buf: []int8{0x20},
			typ: &PerfPyStrType{Type: uint8(PyStrTypeNotCompact), SizeCodepoints: 239},
			res: "",
		},
	}
	for _, testdatum := range testdata {
		t.Run(testdatum.res, func(t *testing.T) {
			res := PythonString(testdatum.buf, testdatum.typ)
			if res != testdatum.res {
				assert.Equal(t, testdatum.res, res)
			}
		})
	}
}
