//go:build !purego

#include "textflag.h"
#include "min_amd64.h"
#include "max_amd64.h"

// func combinedMinMaxBool(data []bool) (min, max bool)
TEXT ·combinedMinMaxBool(SB), NOSPLIT, $-26
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    // Initialize the registers holding the min and max values to false because
    // the input may be empty, in which case the function must return min=false
    // and max=false.
    XORQ R8, R8 // min
    XORQ R9, R9 // max

    CMPQ CX, $0
    JE done
    // At the start, assume min=true and max=false; if we encounter false values
    // the min will be set to false, and vice verso for true and max.
    MOVQ $1, R8
    MOVQ $0, R10 // false
    MOVQ $1, R11 // true
    XORQ SI, SI

    CMPB ·hasAVX512MinMaxBool(SB), $0
    JE loop

    CMPQ CX, $128
    JB loop

    MOVQ DX, DX
    MOVQ CX, DI
    SHRQ $7, DI
    SHLQ $7, DI

    VPXORQ Z0, Z0, Z0
    VPXORQ Z1, Z1, Z1
loop128:
    // This is the same core logic as the maxBool function; we're intersted in
    // knowing if all values were 0 or 1, which determine the three possible
    // states of the result:
    //
    // * min=false max=false
    // * min=false max=true
    // * min=true  max=true
    //
    VMOVDQU64 (AX)(SI*1), Z2
    VMOVDQU64 64(AX)(SI*1), Z3
    VPOPCNTQ Z2, Z2
    VPOPCNTQ Z3, Z3
    VPADDQ Z2, Z0, Z0
    VPADDQ Z3, Z1, Z1
    ADDQ $128, SI
    CMPQ SI, DI
    JNE loop128

    VPADDQ Z1, Z0, Z0

    VMOVDQU64 swap64+0(SB), Z1
    VPERMI2Q Z0, Z0, Z1
    VPADDQ Y1, Y0, Y0

    VMOVDQU64 swap64+32(SB), Y1
    VPERMI2Q Y0, Y0, Y1
    VPADDQ X1, X0, X0

    VMOVDQU64 swap64+48(SB), X1
    VPERMI2Q X0, X0, X1
    VPEXTRQ $1, X1, R12
    VZEROUPPER

    MOVQ X0, R13
    ADDQ R13, R12

    CMPQ R12, $0 // all false?
    CMOVQEQ R10, R8
    CMOVQEQ R10, R9
    CMOVQNE R11, R9
    CMPQ R12, DX // all true?
    CMOVQNE R10, R8
    CMOVQEQ R11, R8
    CMOVQEQ R11, R9

    CMPQ SI, CX
    JE done
loop:
    MOVBQZX (AX)(SI*1), DX
    CMPQ DX, $0
    CMOVQEQ R10, R8
    CMOVQNE R11, R9
    INCQ SI
    CMPQ SI, CX
    JNE loop
done:
    MOVB R8, min+24(FP)
    MOVB R9, max+25(FP)
    RET

// func combinedMinMaxInt32(data []int32) (min, max int32)
TEXT ·combinedMinMaxInt32(SB), NOSPLIT, $-32
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    XORQ R8, R8
    XORQ R9, R9

    CMPQ CX, $0
    JE done
    XORQ SI, SI
    MOVLQZX (AX), R8 // min
    MOVLQZX (AX), R9 // max

    CMPB ·hasAVX512(SB), $0
    JE loop

    CMPQ CX, $32
    JB loop

    MOVQ CX, DI
    SHRQ $5, DI
    SHLQ $5, DI
    VPBROADCASTD (AX), Z0
    VPBROADCASTD (AX), Z3
loop32:
    VMOVDQU32 (AX)(SI*4), Z1
    VMOVDQU32 64(AX)(SI*4), Z2
    VPMINSD Z1, Z0, Z0
    VPMINSD Z2, Z0, Z0
    VPMAXSD Z1, Z3, Z3
    VPMAXSD Z2, Z3, Z3
    ADDQ $32, SI
    CMPQ SI, DI
    JNE loop32

    VMOVDQU32 swap32+0(SB), Z1
    VMOVDQU32 swap32+0(SB), Z2
    VPERMI2D Z0, Z0, Z1
    VPERMI2D Z3, Z3, Z2
    VPMINSD Y1, Y0, Y0
    VPMAXSD Y2, Y3, Y3

    VMOVDQU32 swap32+32(SB), Y1
    VMOVDQU32 swap32+32(SB), Y2
    VPERMI2D Y0, Y0, Y1
    VPERMI2D Y3, Y3, Y2
    VPMINSD X1, X0, X0
    VPMAXSD X2, X3, X3

    VMOVDQU32 swap32+48(SB), X1
    VMOVDQU32 swap32+48(SB), X2
    VPERMI2D X0, X0, X1
    VPERMI2D X3, X3, X2
    VPMINSD X1, X0, X0
    VPMAXSD X2, X3, X3
    VZEROUPPER

    MOVQ X0, BX
    MOVQ X3, DX
    MOVL BX, R8
    MOVL DX, R9
    SHRQ $32, BX
    SHRQ $32, DX
    CMPL BX, R8
    CMOVLLT BX, R8
    CMPL DX, R9
    CMOVLGT DX, R9

    CMPQ SI, CX
    JE done
loop:
    MOVLQZX (AX)(SI*4), DX
    CMPL DX, R8
    CMOVLLT DX, R8
    CMPL DX, R9
    CMOVLGT DX, R9
    INCQ SI
    CMPQ SI, CX
    JNE loop
done:
    MOVL R8, min+24(FP)
    MOVL R9, max+28(FP)
    RET

// func combinedMinMaxInt64(data []int64) (min, max int64)
TEXT ·combinedMinMaxInt64(SB), NOSPLIT, $-40
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    XORQ R8, R8
    XORQ R9, R9

    CMPQ CX, $0
    JE done
    XORQ SI, SI
    MOVQ (AX), R8 // min
    MOVQ (AX), R9 // max

    CMPB ·hasAVX512(SB), $0
    JE loop

    CMPQ CX, $16
    JB loop

    MOVQ CX, DI
    SHRQ $4, DI
    SHLQ $4, DI
    VPBROADCASTQ (AX), Z0
    VPBROADCASTQ (AX), Z3
loop16:
    VMOVDQU64 (AX)(SI*8), Z1
    VMOVDQU64 64(AX)(SI*8), Z2
    VPMINSQ Z1, Z0, Z0
    VPMINSQ Z2, Z0, Z0
    VPMAXSQ Z1, Z3, Z3
    VPMAXSQ Z2, Z3, Z3
    ADDQ $16, SI
    CMPQ SI, DI
    JNE loop16

    VMOVDQU32 swap32+0(SB), Z1
    VMOVDQU32 swap32+0(SB), Z2
    VPERMI2D Z0, Z0, Z1
    VPERMI2D Z3, Z3, Z2
    VPMINSQ Y1, Y0, Y0
    VPMAXSQ Y2, Y3, Y3

    VMOVDQU32 swap32+32(SB), Y1
    VMOVDQU32 swap32+32(SB), Y2
    VPERMI2D Y0, Y0, Y1
    VPERMI2D Y3, Y3, Y2
    VPMINSQ X1, X0, X0
    VPMAXSQ X2, X3, X3

    VMOVDQU32 swap32+48(SB), X1
    VMOVDQU32 swap32+48(SB), X2
    VPERMI2D X0, X0, X1
    VPERMI2D X3, X3, X2
    VPMINSQ X1, X0, X0
    VPMAXSQ X2, X3, X3
    VZEROUPPER

    MOVQ X0, R8
    MOVQ X3, R9
    CMPQ SI, CX
    JE done
loop:
    MOVQ (AX)(SI*8), DX
    CMPQ DX, R8
    CMOVQLT DX, R8
    CMPQ DX, R9
    CMOVQGT DX, R9
    INCQ SI
    CMPQ SI, CX
    JNE loop
done:
    MOVQ R8, min+24(FP)
    MOVQ R9, max+32(FP)
    RET

// func combinedMinMaxUint32(data []uint32) (min, max uint32)
TEXT ·combinedMinMaxUint32(SB), NOSPLIT, $-32
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    XORQ R8, R8
    XORQ R9, R9

    CMPQ CX, $0
    JE done
    XORQ SI, SI
    MOVLQZX (AX), R8 // min
    MOVLQZX (AX), R9 // max

    CMPB ·hasAVX512(SB), $0
    JE loop

    CMPQ CX, $32
    JB loop

    MOVQ CX, DI
    SHRQ $5, DI
    SHLQ $5, DI
    VPBROADCASTD (AX), Z0
    VPBROADCASTD (AX), Z3
loop32:
    VMOVDQU32 (AX)(SI*4), Z1
    VMOVDQU32 64(AX)(SI*4), Z2
    VPMINUD Z1, Z0, Z0
    VPMINUD Z2, Z0, Z0
    VPMAXUD Z1, Z3, Z3
    VPMAXUD Z2, Z3, Z3
    ADDQ $32, SI
    CMPQ SI, DI
    JNE loop32

    VMOVDQU32 swap32+0(SB), Z1
    VMOVDQU32 swap32+0(SB), Z2
    VPERMI2D Z0, Z0, Z1
    VPERMI2D Z3, Z3, Z2
    VPMINUD Y1, Y0, Y0
    VPMAXUD Y2, Y3, Y3

    VMOVDQU32 swap32+32(SB), Y1
    VMOVDQU32 swap32+32(SB), Y2
    VPERMI2D Y0, Y0, Y1
    VPERMI2D Y3, Y3, Y2
    VPMINUD X1, X0, X0
    VPMAXUD X2, X3, X3

    VMOVDQU32 swap32+48(SB), X1
    VMOVDQU32 swap32+48(SB), X2
    VPERMI2D X0, X0, X1
    VPERMI2D X3, X3, X2
    VPMINUD X1, X0, X0
    VPMAXUD X2, X3, X3
    VZEROUPPER

    MOVQ X0, BX
    MOVQ X3, DX
    MOVL BX, R8
    MOVL DX, R9
    SHRQ $32, BX
    SHRQ $32, DX
    CMPL BX, R8
    CMOVLCS BX, R8
    CMPL DX, R9
    CMOVLHI DX, R9

    CMPQ SI, CX
    JE done
loop:
    MOVLQZX (AX)(SI*4), DX
    CMPL DX, R8
    CMOVLCS DX, R8
    CMPL DX, R9
    CMOVLHI DX, R9
    INCQ SI
    CMPQ SI, CX
    JNE loop
done:
    MOVL R8, min+24(FP)
    MOVL R9, max+28(FP)
    RET

// func combinedMinMaxUint64(data []uint64) (min, max uint64)
TEXT ·combinedMinMaxUint64(SB), NOSPLIT, $-40
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    XORQ R8, R8
    XORQ R9, R9

    CMPQ CX, $0
    JE done
    XORQ SI, SI
    MOVQ (AX), R8 // min
    MOVQ (AX), R9 // max

    CMPB ·hasAVX512(SB), $0
    JE loop

    CMPQ CX, $16
    JB loop

    MOVQ CX, DI
    SHRQ $4, DI
    SHLQ $4, DI
    VPBROADCASTQ (AX), Z0
    VPBROADCASTQ (AX), Z3
loop16:
    VMOVDQU64 (AX)(SI*8), Z1
    VMOVDQU64 64(AX)(SI*8), Z2
    VPMINUQ Z1, Z0, Z0
    VPMINUQ Z2, Z0, Z0
    VPMAXUQ Z1, Z3, Z3
    VPMAXUQ Z2, Z3, Z3
    ADDQ $16, SI
    CMPQ SI, DI
    JNE loop16

    VMOVDQU32 swap32+0(SB), Z1
    VMOVDQU32 swap32+0(SB), Z2
    VPERMI2D Z0, Z0, Z1
    VPERMI2D Z3, Z3, Z2
    VPMINUQ Y1, Y0, Y0
    VPMAXUQ Y2, Y3, Y3

    VMOVDQU32 swap32+32(SB), Y1
    VMOVDQU32 swap32+32(SB), Y2
    VPERMI2D Y0, Y0, Y1
    VPERMI2D Y3, Y3, Y2
    VPMINUQ X1, X0, X0
    VPMAXUQ X2, X3, X3

    VMOVDQU32 swap32+48(SB), X1
    VMOVDQU32 swap32+48(SB), X2
    VPERMI2D X0, X0, X1
    VPERMI2D X3, X3, X2
    VPMINUQ X1, X0, X0
    VPMAXUQ X2, X3, X3
    VZEROUPPER

    MOVQ X0, R8
    MOVQ X3, R9
    CMPQ SI, CX
    JE done
loop:
    MOVQ (AX)(SI*8), DX
    CMPQ DX, R8
    CMOVQCS DX, R8
    CMPQ DX, R9
    CMOVQHI DX, R9
    INCQ SI
    CMPQ SI, CX
    JNE loop
done:
    MOVQ R8, min+24(FP)
    MOVQ R9, max+32(FP)
    RET

// func combinedMinMaxFloat32(data []float32) (min, max float32)
TEXT ·combinedMinMaxFloat32(SB), NOSPLIT, $-32
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    XORQ R8, R8
    XORQ R9, R9

    CMPQ CX, $0
    JE done
    XORPS X0, X0
    XORPS X1, X1
    XORQ SI, SI
    MOVLQZX (AX), R8 // min
    MOVLQZX (AX), R9 // max
    MOVQ R8, X0
    MOVQ R9, X1

    CMPB ·hasAVX512(SB), $0
    JE loop

    CMPQ CX, $32
    JB loop

    MOVQ CX, DI
    SHRQ $5, DI
    SHLQ $5, DI
    VPBROADCASTD (AX), Z0
    VPBROADCASTD (AX), Z3
loop32:
    VMOVDQU32 (AX)(SI*4), Z1
    VMOVDQU32 64(AX)(SI*4), Z2
    VMINPS Z1, Z0, Z0
    VMINPS Z2, Z0, Z0
    VMAXPS Z1, Z3, Z3
    VMAXPS Z2, Z3, Z3
    ADDQ $32, SI
    CMPQ SI, DI
    JNE loop32

    VMOVDQU32 swap32+0(SB), Z1
    VMOVDQU32 swap32+0(SB), Z2
    VPERMI2D Z0, Z0, Z1
    VPERMI2D Z3, Z3, Z2
    VMINPS Y1, Y0, Y0
    VMAXPS Y2, Y3, Y3

    VMOVDQU32 swap32+32(SB), Y1
    VMOVDQU32 swap32+32(SB), Y2
    VPERMI2D Y0, Y0, Y1
    VPERMI2D Y3, Y3, Y2
    VMINPS X1, X0, X0
    VMAXPS X2, X3, X3

    VMOVDQU32 swap32+48(SB), X1
    VMOVDQU32 swap32+48(SB), X2
    VPERMI2D X0, X0, X1
    VPERMI2D X3, X3, X2
    VMINPS X1, X0, X0
    VMAXPS X2, X3, X3
    VZEROUPPER

    MOVAPS X0, X1
    MOVAPS X3, X2

    PSRLQ $32, X1
    MOVQ X0, R8
    MOVQ X1, R10
    UCOMISS X0, X1
    CMOVLCS R10, R8

    PSRLQ $32, X2
    MOVQ X3, R9
    MOVQ X2, R11
    UCOMISS X3, X2
    CMOVLHI R11, R9

    CMPQ SI, CX
    JE done
    MOVQ R8, X0
    MOVQ R9, X1
loop:
    MOVLQZX (AX)(SI*4), DX
    MOVQ DX, X2
    UCOMISS X0, X2
    CMOVLCS DX, R8
    UCOMISS X1, X2
    CMOVLHI DX, R9
    MOVQ R8, X0
    MOVQ R9, X1
    INCQ SI
    CMPQ SI, CX
    JNE loop
done:
    MOVL R8, min+24(FP)
    MOVL R9, max+28(FP)
    RET

// func combinedMinMaxFloat64(data []float64) (min, max float64)
TEXT ·combinedMinMaxFloat64(SB), NOSPLIT, $-40
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    XORQ R8, R8
    XORQ R9, R9

    CMPQ CX, $0
    JE done
    XORPD X0, X0
    XORPD X1, X1
    XORQ SI, SI
    MOVQ (AX), R8 // min
    MOVQ (AX), R9 // max
    MOVQ R8, X0
    MOVQ R9, X1

    CMPB ·hasAVX512(SB), $0
    JE loop

    CMPQ CX, $16
    JB loop

    MOVQ CX, DI
    SHRQ $4, DI
    SHLQ $4, DI
    VPBROADCASTQ (AX), Z0
    VPBROADCASTQ (AX), Z3
loop16:
    VMOVDQU64 (AX)(SI*8), Z1
    VMOVDQU64 64(AX)(SI*8), Z2
    VMINPD Z1, Z0, Z0
    VMINPD Z2, Z0, Z0
    VMAXPD Z1, Z3, Z3
    VMAXPD Z2, Z3, Z3
    ADDQ $16, SI
    CMPQ SI, DI
    JNE loop16

    VMOVDQU64 swap32+0(SB), Z1
    VMOVDQU64 swap32+0(SB), Z2
    VPERMI2D Z0, Z0, Z1
    VPERMI2D Z3, Z3, Z2
    VMINPD Y1, Y0, Y0
    VMAXPD Y2, Y3, Y3

    VMOVDQU64 swap32+32(SB), Y1
    VMOVDQU64 swap32+32(SB), Y2
    VPERMI2D Y0, Y0, Y1
    VPERMI2D Y3, Y3, Y2
    VMINPD X1, X0, X0
    VMAXPD X2, X3, X3

    VMOVDQU64 swap32+48(SB), X1
    VMOVDQU64 swap32+48(SB), X2
    VPERMI2D X0, X0, X1
    VPERMI2D X3, X3, X2
    VMINPD X1, X0, X0
    VMAXPD X2, X3, X1
    VZEROUPPER

    MOVQ X0, R8
    MOVQ X1, R9
    CMPQ SI, CX
    JE done
loop:
    MOVQ (AX)(SI*8), DX
    MOVQ DX, X2
    UCOMISD X0, X2
    CMOVQCS DX, R8
    UCOMISD X1, X2
    CMOVQHI DX, R9
    MOVQ R8, X0
    MOVQ R9, X1
    INCQ SI
    CMPQ SI, CX
    JNE loop
done:
    MOVQ R8, min+24(FP)
    MOVQ R9, max+32(FP)
    RET

// func combinedMinMaxBE128(data []byte) (min, max []byte)
TEXT ·combinedMinMaxBE128(SB), NOSPLIT, $-72
    MOVQ data_base+0(FP), AX
    MOVQ data_len+8(FP), CX
    CMPQ CX, $0
    JE null
    MOVQ AX, R8 // min
    MOVQ AX, R9 // max
    ADDQ AX, CX // end
loop:
    MOVBEQQ (AX), R10
    MOVBEQQ (R8), R11
    MOVBEQQ (R9), R12
    CMPQ R10, R11
    JA testmax
    JB setmin
    MOVBEQQ 8(AX), R13
    MOVBEQQ 8(R8), R11
    CMPQ R11, R13
    JAE testmax
setmin:
    MOVQ AX, R8
testmax:
    CMPQ R10, R12
    JA setmax
    JB next
    MOVBEQQ 8(AX), R13
    MOVBEQQ 8(R9), R12
    CMPQ R12, R13
    JAE next
setmax:
    MOVQ AX, R9
next:
    ADDQ $16, AX
    CMPQ AX, CX
    JNE loop
done:
    MOVQ R8, min+24(FP)
    MOVQ $16, min+32(FP)
    MOVQ $16, min+40(FP)
    MOVQ R9, max+48(FP)
    MOVQ $16, max+56(FP)
    MOVQ $16, max+64(FP)
    RET
null:
    VPXOR X0, X0, X0
    VMOVDQU X0, ret+24(FP)
    VMOVDQU X0, ret+40(FP)
    VMOVDQU X0, ret+56(FP)
    RET
