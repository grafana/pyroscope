package util

import (
	"unsafe"

	"github.com/oklog/ulid/v2"
)

func ULIDStringUnixNano(s string) int64 {
	var u ulid.ULID
	b := unsafe.Slice(unsafe.StringData(s), len(s))
	if err := u.UnmarshalText(b); err == nil {
		return int64(u.Time()) * 1e6
	}
	return -1
}
