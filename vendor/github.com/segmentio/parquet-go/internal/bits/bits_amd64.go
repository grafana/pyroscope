package bits

import (
	"golang.org/x/sys/cpu"
)

// All functions require at least the F and VL instruction sets (Foundation and
// Vector Length extensions). Note that Foundation is a requirement of AVX-512,
// and usually includes Vector Length extensions as well, so we could simply be
// testing HasAVX512 but we leave the other checks to be more explicit about our
// intentions, and maybe be more prepared for unconventional implementations.
var hasAVX512 = cpu.X86.HasAVX512 &&
	cpu.X86.HasAVX512F &&
	cpu.X86.HasAVX512VL

// For min/max functions over big-endian 128 bits values, we need the follwing
// instructions from the DQ set:
// * VPBROADCASTQ (with 64 bits source register)
// * VBROADCASTI64X2
var hasAVX512MinMaxBE128 = hasAVX512 &&
	cpu.X86.HasAVX512DQ

// For min/max functions over boolean values, we need the following instructions
// from the VPOPCNTDQ set:
// * VPOPCNTDQ
var hasAVX512MinMaxBool = hasAVX512 &&
	cpu.X86.HasAVX512VPOPCNTDQ

// These use AVX-512 instructions in the minBool algorithm relies operations
// that are avilable in the AVX512BW extension:
// * VPCMPUB
// * KMOVQ
var hasAVX512MinBool = hasAVX512 &&
	cpu.X86.HasAVX512BW

// These use AVX-512 instructions in the maxBool algorithm relies operations
// that are avilable in the AVX512BW extension:
// * VPCMPUB
// * KMOVQ
var hasAVX512MaxBool = hasAVX512 &&
	cpu.X86.HasAVX512BW

// These use AVX-512 instructions in the countByte algorithm relies operations
// that are avilable in the AVX512BW extension:
// * VPCMPUB
// * KMOVQ
//
// Note that the function will fallback to an AVX2 version if those instructions
// are not available.
var hasAVX512CountByte = hasAVX512 &&
	cpu.X86.HasAVX512BW
