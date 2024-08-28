//go:build arm64 && linux

package python

// https://github.com/bminor/glibc/blob/49b308a26e2a9e02ef396f67f59c462ad4171ea4/sysdeps/aarch64/nptl/bits/pthreadtypes-arch.h#L34C24-L34C24
// # define __SIZEOF_PTHREAD_MUTEX_T       48
const mutexSizeGlibc = 48

// https://github.com/bminor/musl/blob/f314e133929b6379eccc632bef32eaebb66a7335/include/alltypes.h.in#L86C1-L86C173
const mutexSizeMusl = 40
