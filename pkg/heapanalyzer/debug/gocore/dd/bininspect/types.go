// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package bininspect provides tools to inspect a Go binary.
package bininspect

import (
	"debug/elf"
	"errors"
	"reflect"

	"github.com/go-delve/delve/pkg/goversion"
)

const (
	// WriteGoTLSFunc is the name of the function that writes to the TLS connection.
	WriteGoTLSFunc = "crypto/tls.(*Conn).Write"
	// ReadGoTLSFunc is the name of the function that reads from the TLS connection.
	ReadGoTLSFunc = "crypto/tls.(*Conn).Read"
	// CloseGoTLSFunc is the name of the function that closes the TLS connection.
	CloseGoTLSFunc = "crypto/tls.(*Conn).Close"
)

// StructOffsetTLSConn is the offset of the `conn` field within `crypto/tls.Conn`.
var StructOffsetTLSConn = FieldIdentifier{
	StructName: "crypto/tls.Conn",
	FieldName:  "conn",
}

// StructOffsetTCPConn is the offset of the `conn` field within `net.TCPConn`.
var StructOffsetTCPConn = FieldIdentifier{
	StructName: "net.TCPConn",
	FieldName:  "conn",
}

// StructOffsetNetConnFd is the offset of the `fd` field within `net.conn`.
var StructOffsetNetConnFd = FieldIdentifier{
	StructName: "net.conn",
	FieldName:  "fd",
}

// StructOffsetNetFdPfd is the offset of the `pdf` field within `net.netFD`.
var StructOffsetNetFdPfd = FieldIdentifier{
	StructName: "net.netFD",
	FieldName:  "pfd",
}

// StructOffsetPollFdSysfd is the offset of the `sysfd` field within `internal/poll.FD`.
var StructOffsetPollFdSysfd = FieldIdentifier{
	StructName: "internal/poll.FD",
	FieldName:  "Sysfd",
}

type ElfMetadata struct {
	File *elf.File
	Arch GoArch
}

// Result is the result of the binary inspection process.
type Result struct {
	Arch                GoArch
	ABI                 GoABI
	GoVersion           goversion.GoVersion
	Functions           map[string]FunctionMetadata
	StructOffsets       map[FieldIdentifier]uint64
	GoroutineIDMetadata GoroutineIDMetadata
}

// GoArch only includes supported architectures,
// and the underlying values are compatible with GOARCH values.
type GoArch string

const (
	// GoArchX86_64 corresponds to x86 64-bit ("amd64")
	GoArchX86_64 GoArch = "amd64"
	// GoArchARM64 corresponds to ARM 64-bit ("arm64")
	GoArchARM64 GoArch = "arm64"
)

// PointerSize gets the size, in bytes, of pointers on the architecture
func (a *GoArch) PointerSize() uint {
	switch *a {
	case GoArchX86_64:
		return 8
	case GoArchARM64:
		return 8
	default:
		return 0
	}
}

// GoABI is the type of ABI used by the Go compiler when generating a binary
type GoABI string

const (
	// GoABIStack is the same as abi0
	GoABIStack GoABI = "stack"
	// GoABIRegister is the same as abi internal (as of 2021-11-12)
	GoABIRegister GoABI = "register"
)

// FunctionMetadata contains the results of the inspection process that
// that can be used to attach an eBPF uprobe to a single function.
type FunctionMetadata struct {
	// The virtual address for the function's entrypoint for non-PIE's,
	// or offset to the file load address for PIE's.
	//
	// This should really probably be the location of the end of the prologue
	// (which might help with parameter locations being half-spilled),
	// but so far using the first PC position in the function has worked
	// for the functions we're tracing.
	// See:
	// - https://github.com/go-delve/delve/pull/2704#issuecomment-944374511
	//   (which implies that the instructions in the prologue
	//   might get executed multiple times over the course of a single function call,
	//   though I'm not sure under what circumstances this might be true)
	EntryLocation uint64
	// A list of parameter metadata structs
	// used for extracting the function arguments in the eBPF probe.
	//
	// When optimizations are enabled, and depending on the Go version,
	// it has been observed that these parameter locations are sometimes missing pieces
	// (either due to them being eliminated on purpose
	// (as in the case of an unused slice length/capacity/data) being omitted)
	// or due to (spooky behavior in the compiler?))
	// or missing entire parameters (again due to (spooky behavior in the compiler?)).
	// This is unfortunate; I don't think there is a clean way to work around this
	// (since the data is just missing).
	// The best solution I have found is to:
	// - manually handle cases where entire parameters are missing
	//   if it's known that they only occur on a specific Go version
	//   (Go 1.16.* is one that I have found to be troublesome)
	//   and insert the appropriate parameter locations if known.
	// - if all of the parameters are missing,
	//   then I have found that manually re-implementing
	//   the stack/register allocation algorithm for function arguments/returns
	//   (depending on the ABI, and using the sizes of the values from the type metadata)
	//   can work to still obtain the parameters at runtime.
	//   This is probably much more brittle
	//   than getting the locations directly from the debug information.
	//
	// If the outer result's IncludesDebugSymbols is false,
	// then this slice always will be nil/empty.
	Parameters []ParameterMetadata
	// A list of locations for each return instruction in the compiled function machine code.
	// Each element is the virtual address for the function's entrypoint for non-PIE's,
	// or offset to the file load address for PIE's.
	// Only given if `IncludesReturn` is true.
	//
	// Note that this may not behave well with panics or defer statements.
	ReturnLocations []uint64
}

// ParameterMetadata contains information about a single parameter
// (both traditional input parameters and return "parameters").
// This includes both data about the type of the parameter
// (and its total size, in bytes)
// as well as the position of the parameter (or its pieces)
// at runtime, which will either be on the stack or in registers.
//
// See the note on FunctionMetadata.Parameters
// for limitations of the position data included within this struct.
//
// Note that while Go only ever passes parameters
// entirely in the stack or entirely in registers
// (depending on the ABI used and the version of the Go compiler),
// it has been observed that the position metadata emitted by the compiler
// at the function's entrypoint indicates partially-spilled parameters
// that have some pieces still in registers
// and some pieces that have been moved into their reserved space on the stack.
// It's a little unclear whether the location metadata is *correct* in these cases
// (i.e. when attaching an eBPF uprobe at the entrypoint of the function,
// has the first instruction to spill the registers actually been executed?),
// but so far, taking the locations at face value and interpreting them directly
// (specifically handling mixed stack/register locations)
// has been successful in getting the expected values from eBPF.
//
// TODO: look into cases where a middle piece of a parameter has been eliminated
//
//	(such as the length of a slice),
//	and make sure result is expected/handled well.
//	Then, document such cases.
type ParameterMetadata struct {
	// Total size in bytes.
	TotalSize int64
	// Kind of variable.
	Kind reflect.Kind

	// Pieces that make up the location of this parameter at runtime
	Pieces []ParameterPiece
}

// ParameterPiece represents a single piece of a parameter
// that either exists entirely on the stack or in a register.
// This might be the full parameter, or might be just part of it
// (especially in the case of slices/interfaces/strings
// that are made up of multiple word-sized parts).
type ParameterPiece struct {
	// Size of this piece in bytes
	Size int64
	// True if this piece is contained in a register.
	InReg bool
	// Offset from the stackpointer.
	// Only given if the piece resides on the stack.
	StackOffset int64
	// Register number of the piece.
	// Only given if the piece resides in registers.
	Register int
}

// FieldIdentifier represents a field in a struct and can be used as a map key.
type FieldIdentifier struct {
	// Fully-qualified name of the struct
	StructName string
	// Name of the field in the struct
	FieldName string
}

// GoroutineIDMetadata contains information
// that can be used to reliably determine the ID
// of the currently-running goroutine from an eBPF uprobe.
type GoroutineIDMetadata struct {
	// The offset of the `goid` field within `runtime.g`
	GoroutineIDOffset uint64
	// Whether the pointer to the current `runtime.g` value is in a register.
	// If true, then `RuntimeGRegister` is given.
	// Otherwise, `RuntimeGTLSAddrOffset` is given
	RuntimeGInRegister bool

	// The register that the pointer to the current `runtime.g` is in.
	RuntimeGRegister int
	// The offset of the `runtime.g` value within thread-local-storage.
	RuntimeGTLSAddrOffset uint64
}

// ParameterLookupFunction represents a function that returns a list of parameter metadata (for example, parameter size)
// for a specific golang function. It selects the relevant parameters metadata by the given go version & architecture.
type ParameterLookupFunction func(goversion.GoVersion, string) ([]ParameterMetadata, error)

// StructLookupFunction represents a function that returns the offset of a specific field in a struct.
// It selects the relevant offset metadata by the given go version & architecture.
type StructLookupFunction func(goversion.GoVersion, string) (uint64, error)

// FunctionConfiguration contains info for the function analyzing process when scanning a binary.
type FunctionConfiguration struct {
	IncludeReturnLocations bool
	ParamLookupFunction    ParameterLookupFunction
}

// ErrNilElf is returned when getting a nil pointer to an elf file.
var ErrNilElf = errors.New("got nil elf file")

// ErrUnsupportedArch is returned when an architecture given as a parameter is not supported.
var ErrUnsupportedArch = errors.New("got unsupported arch")
