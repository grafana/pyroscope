//go:build !purego

#include "textflag.h"

#define bswap128lo 0x08080A0B0C0D0E0F
#define bswap128hi 0x0001020304050607

DATA bswap128+0(SB)/8, $bswap128lo
DATA bswap128+8(SB)/8, $bswap128hi
DATA bswap128+16(SB)/8, $bswap128lo
DATA bswap128+24(SB)/8, $bswap128hi
DATA bswap128+32(SB)/8, $bswap128lo
DATA bswap128+40(SB)/8, $bswap128hi
DATA bswap128+48(SB)/8, $bswap128lo
DATA bswap128+56(SB)/8, $bswap128hi
GLOBL bswap128(SB), RODATA|NOPTR, $64

DATA indexes128+0(SB)/8, $0
DATA indexes128+8(SB)/8, $0
DATA indexes128+16(SB)/8, $1
DATA indexes128+24(SB)/8, $1
DATA indexes128+32(SB)/8, $2
DATA indexes128+40(SB)/8, $2
DATA indexes128+48(SB)/8, $3
DATA indexes128+56(SB)/8, $3
GLOBL indexes128(SB), RODATA|NOPTR, $64

DATA swap64+0(SB)/8, $4
DATA swap64+8(SB)/8, $5
DATA swap64+16(SB)/8, $6
DATA swap64+24(SB)/8, $7
DATA swap64+32(SB)/8, $2
DATA swap64+40(SB)/8, $3
DATA swap64+48(SB)/8, $0
DATA swap64+56(SB)/8, $1
GLOBL swap64(SB), RODATA|NOPTR, $64

DATA swap32+0(SB)/4, $8
DATA swap32+4(SB)/4, $9
DATA swap32+8(SB)/4, $10
DATA swap32+12(SB)/4, $11
DATA swap32+16(SB)/4, $12
DATA swap32+20(SB)/4, $13
DATA swap32+24(SB)/4, $14
DATA swap32+28(SB)/4, $15
DATA swap32+32(SB)/4, $4
DATA swap32+36(SB)/4, $5
DATA swap32+40(SB)/4, $6
DATA swap32+44(SB)/4, $7
DATA swap32+48(SB)/4, $2
DATA swap32+52(SB)/4, $3
DATA swap32+56(SB)/4, $0
DATA swap32+60(SB)/4, $1
GLOBL swap32(SB), RODATA|NOPTR, $64
