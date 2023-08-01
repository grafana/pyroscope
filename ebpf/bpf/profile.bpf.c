// SPDX-License-Identifier: GPL-2.0-only

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "profile.bpf.h"
#include "pyperf.bpf.c"





struct bss_arg arg2;




SEC("perf_event")
int do_perf_event(struct bpf_perf_event_data *ctx)
{
    u64 id = bpf_get_current_pid_tgid();
    u32 tgid = id >> 32;
    u32 pid = id;
	struct sample_key key = {};
	u32 *val, one = 1, zero = 0;

	struct bss_arg *arg = bpf_map_lookup_elem(&args, &zero);
    if (!arg) {
        return 0;
    }
    if (pid == 0) {
        return 0;
    }

    // this will not return if it is python
    if (pyperf_collect_impl(ctx, (pid_t)tgid, arg->collect_kernel) < 0) {
        return 0;
    }


    key.pid = tgid;
    key.kern_stack = -1;
    key.user_stack = -1;



	if (arg->collect_kernel) {
	    key.kern_stack = bpf_get_stackid(ctx, &stacks, KERN_STACKID_FLAGS);
	}
	if (arg->collect_user)  {
	    key.user_stack = bpf_get_stackid(ctx, &stacks, USER_STACKID_FLAGS);
	}

	val = bpf_map_lookup_elem(&counts, &key);
	if (val)
		(*val)++;
	else
		bpf_map_update_elem(&counts, &key, &one, BPF_NOEXIST);
	return 0;
}

char _license[] SEC("license") = "GPL";
