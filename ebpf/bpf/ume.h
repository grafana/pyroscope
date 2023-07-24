#if !defined(UME_H)
#define UME_H


#if defined(PYROSCOPE_UME)

#define pyro_bpf_core_read(dst, sz, src)					    \
		bpf_probe_read_kernel(dst, sz, src)

#define pyro_bpf_tail_call(ctx, prog_array_map, index) \
    return (*prog_array_map).values[(index)]((ctx));

#else

#include "bpf_core_read.h"

#define pyro_bpf_core_read(dst, sz, src)					    \
	bpf_core_read(dst, sz, src)

#define pyro_bpf_tail_call(ctx, prog_array_map, index) \
    bpf_tail_call(ctx, prog_array_map, index)

#endif

#endif // UME_H