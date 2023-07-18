package internal

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

type MapType uint32
type TypeID uint32
type MapFlags uint32
type ObjName [unix.BPF_OBJ_NAME_LEN]byte
type Cmd uint32

const (
	BPF_MAP_CREATE Cmd = 0
)

type MapCreateAttr struct {
	MapType               MapType
	KeySize               uint32
	ValueSize             uint32
	MaxEntries            uint32
	MapFlags              MapFlags
	InnerMapFd            uint32
	NumaNode              uint32
	MapName               ObjName
	MapIfindex            uint32
	BtfFd                 uint32
	BtfKeyTypeId          TypeID
	BtfValueTypeId        TypeID
	BtfVmlinuxValueTypeId TypeID
	MapExtra              uint64
}

func MapCreate(attr *MapCreateAttr) (uintptr, error) {
	var attr2 = unsafe.Pointer(attr)
	r1, _, errNo := unix.Syscall(unix.SYS_BPF, uintptr(BPF_MAP_CREATE), uintptr(attr2), unsafe.Sizeof(*attr))
	runtime.KeepAlive(attr2)
	if errNo == 0 {
		return r1, nil
	}
	return r1, errNo
}
