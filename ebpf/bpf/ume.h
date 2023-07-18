#ifndef UME_H
#define UME_H


#if defined(PYROSCOPE_UME)

#define pyro_bpf_core_read(dst, sz, src)					    \
		bpf_probe_read_kernel(dst, sz, src)


#else

#include "bpf_core_read.h"

#define pyro_bpf_core_read(dst, sz, src)					    \
	bpf_core_read(dst, sz, src)


#endif

#endif // UME_H