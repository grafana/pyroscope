#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include "profile.bpf.h"

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct profile_key_t);
	__type(value, u32);
	__uint(max_entries, PROFILE_MAPS_SIZE);
} counts SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_STACK_TRACE);
	__uint(key_size, sizeof(u32));
	__uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
	__uint(max_entries, PROFILE_MAPS_SIZE);
} stacks SEC(".maps");

#define KERN_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP)
#define USER_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP | BPF_F_USER_STACK)

struct profile_bss_args_t args;

SEC("perf_event")
int do_perf_event(struct bpf_perf_event_data *ctx)
{
    u64 id = bpf_get_current_pid_tgid();
    u32 tgid = id >> 32;
    u32 pid = id;
	struct profile_key_t key = {};
	if (args.use_tgid_as_key) {
	    key.pid = tgid;
	} else {
	    key.pid = pid;
	}
	u32 *val, one = 1;

    if (pid == 0) {
        return 0;
    }
    if (args.tgid_filter != 0 && tgid != args.tgid_filter) {
        return 0;
    }
    if (args.use_comm) {
	    bpf_get_current_comm(&key.comm, sizeof(key.comm));
    }
	key.kern_stack = bpf_get_stackid(ctx, &stacks, KERN_STACKID_FLAGS);
	key.user_stack = bpf_get_stackid(ctx, &stacks, USER_STACKID_FLAGS);

	val = bpf_map_lookup_elem(&counts, &key);
	if (val)
		(*val)++;
	else
		bpf_map_update_elem(&counts, &key, &one, BPF_NOEXIST);
	return 0;
}


char _license[] SEC("license") = "GPL"; //todo