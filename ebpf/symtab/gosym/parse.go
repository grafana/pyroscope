package gosym

import "encoding/binary"

func ParseRuntimeTextFromPclntab18(pclntab []byte) uint64 {
	if len(pclntab) < 64 {
		return 0
	}
	magic := binary.LittleEndian.Uint32(pclntab[0:4])
	if magic == 0xFFFFFFF0 || magic == 0xFFFFFFF1 {
		// https://github.com/golang/go/blob/go1.18/src/runtime/symtab.go#L395
		// 0xFFFFFFF1 is the same
		// https://github.com/golang/go/commit/0f8dffd0aa71ed996d32e77701ac5ec0bc7cde01
		//type pcHeader struct {
		//	magic          uint32  // 0xFFFFFFF0
		//	pad1, pad2     uint8   // 0,0
		//	minLC          uint8   // min instruction size
		//	ptrSize        uint8   // size of a ptr in bytes
		//	nfunc          int     // number of functions in the module
		//	nfiles         uint    // number of entries in the file tab
		//	textStart      uintptr // base for function entry PC offsets in this module, equal to moduledata.text
		//	funcnameOffset uintptr // offset to the funcnametab variable from pcHeader
		//	cuOffset       uintptr // offset to the cutab variable from pcHeader
		//	filetabOffset  uintptr // offset to the filetab variable from pcHeader
		//	pctabOffset    uintptr // offset to the pctab variable from pcHeader
		//	pclnOffset     uintptr // offset to the pclntab variable from pcHeader
		//}
		textStart := binary.LittleEndian.Uint64(pclntab[24:32])
		return textStart
	}

	return 0
}
