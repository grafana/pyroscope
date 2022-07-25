#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define PERF_MAX_STACK_DEPTH         127

struct key_t {
	u32 pid;
	s64 kernstack;
	s64 userstack;
	char comm[16];
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, struct key_t);
	__type(value, u64);
	__uint(max_entries, 16384);//todo sizes
} counts SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_STACK_TRACE);
	__uint(key_size, sizeof(u32));
	__uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
	__uint(max_entries, 16384);//todo sizes
} stacks SEC(".maps");

#define KERN_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP)
#define USER_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP | BPF_F_USER_STACK)



SEC("perf_event")
int do_perf_event(struct bpf_perf_event_data *ctx)
{
    u64 id = bpf_get_current_pid_tgid();
//    u32 tgid = id >> 32;
    u32 pid = id;
	struct key_t key = {.pid = pid};
	u64 *val, one = 1;

    if (pid == 0) {
        return 0;
    }


	bpf_get_current_comm(&key.comm, sizeof(key.comm));
	key.kernstack = bpf_get_stackid(ctx, &stacks, KERN_STACKID_FLAGS);
	key.userstack = bpf_get_stackid(ctx, &stacks, USER_STACKID_FLAGS);

	val = bpf_map_lookup_elem(&counts, &key);
	if (val)
		(*val)++;
	else
		bpf_map_update_elem(&counts, &key, &one, BPF_NOEXIST);
	return 0;
}

char _license[] SEC("license") = "GPL"; //todo
