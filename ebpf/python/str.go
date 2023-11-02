package python

func PythonString(tok []int8, typ *PerfPyStrType) string {
	if typ.Type&uint8(PyStrTypeAscii) != 0 || typ.Type&uint8(PyStrTypeUtf8) != 0 {
		sz := int(typ.SizeCodepoints) // 128 max
		if sz > len(tok) {
			sz = len(tok)
		}
		buf := make([]byte, sz)
		for i := 0; i < sz; i++ {
			buf[i] = byte(tok[i])
		}
		return string(buf)
	}

	if typ.Type&uint8(PyStrType1Byte) != 0 {
		sz := int(typ.SizeCodepoints) // 128 max
		if sz > len(tok) {
			sz = len(tok)
		}
		buf := make([]rune, sz)
		for i := 0; i < sz; i++ {
			buf[i] = rune(uint8(tok[i]))
		}
		return string(buf)
	}
	if typ.Type&uint8(PyStrType2Byte) != 0 {
		sz := int(typ.SizeCodepoints) // 128 max
		if sz*2 > len(tok) {
			sz = len(tok) / 2
		}
		buf := make([]rune, sz)
		for i := 0; i < sz; i++ {
			r := uint16(tok[i*2]) | uint16(tok[i*2+1])<<8
			buf[i] = rune(r)
		}
		return string(buf)
	}
	if typ.Type&uint8(PyStrType4Byte) != 0 {
		sz := int(typ.SizeCodepoints) // 128 max
		if sz*4 > len(tok) {
			sz = len(tok) / 4
		}
		buf := make([]rune, sz)
		for i := 0; i < sz; i++ {
			r := uint32(uint8(tok[i*4])) | uint32(uint8(tok[i*4+1]))<<8 | uint32(uint8(tok[i*4+2]))<<16 | uint32(uint8(tok[i*4+3]))<<24
			buf[i] = rune(r)
		}
		return string(buf)
	}
	return ""
}
