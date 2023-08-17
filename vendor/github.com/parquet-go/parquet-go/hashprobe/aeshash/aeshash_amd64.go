//go:build !purego

package aeshash

import (
	"github.com/parquet-go/parquet-go/sparse"
	"golang.org/x/sys/cpu"
)

// Enabled returns true if AES hash is available on the system.
//
// The function uses the same logic than the Go runtime since we depend on
// the AES hash state being initialized.
//
// See https://go.dev/src/runtime/alg.go
func Enabled() bool { return cpu.X86.HasAES && cpu.X86.HasSSSE3 && cpu.X86.HasSSE41 }

//go:noescape
func Hash32(value uint32, seed uintptr) uintptr

//go:noescape
func Hash64(value uint64, seed uintptr) uintptr

//go:noescape
func Hash128(value [16]byte, seed uintptr) uintptr

//go:noescape
func MultiHashUint32Array(hashes []uintptr, values sparse.Uint32Array, seed uintptr)

//go:noescape
func MultiHashUint64Array(hashes []uintptr, values sparse.Uint64Array, seed uintptr)

//go:noescape
func MultiHashUint128Array(hashes []uintptr, values sparse.Uint128Array, seed uintptr)
