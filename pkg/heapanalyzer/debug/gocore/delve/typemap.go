package delve

import (
	"debug/dwarf"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
)

// using AttrGoRuntimeType to map from _type to Type
// https://github.com/go-delve/delve/blob/938cb6e9d8e59f358d44590a4beb13a98c5a22dc/pkg/proc/types.go#L100

type RuntimeTypeDIE struct {
	Offset dwarf.Offset
	Kind   int64
}

type TypeMap struct {
	// runtimeTypeToDIE maps between the offset of a runtime._type in
	// runtime.moduledata.types and the offset of the DIE in debug_info. This
	// map is filled by using the extended attribute godwarf.AttrGoRuntimeType
	// which was added in go 1.11.
	RuntimeTypeToDIE map[uint64]RuntimeTypeDIE
	TypeCache        map[dwarf.Offset]godwarf.Type
}

func (image *TypeMap) RegisterRuntimeTypeToDIE(entry *dwarf.Entry) {
	if off, ok := entry.Val(godwarf.AttrGoRuntimeType).(uint64); ok {
		if _, ok := image.RuntimeTypeToDIE[off]; !ok {
			image.RuntimeTypeToDIE[off] = RuntimeTypeDIE{entry.Offset, -1}
		}
	}
}
