#ifndef PYROSCOPE_STACKS_H
#define PYROSCOPE_STACKS_H

#define PERF_MAX_STACK_DEPTH      127
#define PROFILE_MAPS_SIZE         16384

#define KERN_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP)
#define USER_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP | BPF_F_USER_STACK)

#define SAMPLE_KEY_FLAG_PYTHON_STACK 1
#define SAMPLE_KEY_FLAG_STACK_TRUNCATED 2

struct sample_key {
    __u32 pid;
    __u32 flags;
    __s64 kern_stack;
    __s64 user_stack;
};

struct {
    __uint(type, BPF_MAP_TYPE_STACK_TRACE);
    __uint(key_size, sizeof(u32));
    __uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
    __uint(max_entries, PROFILE_MAPS_SIZE);
} stacks SEC(".maps");


struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct sample_key);
    __type(value, u32);
    __uint(max_entries, PROFILE_MAPS_SIZE);
} counts SEC(".maps");



#define OP_REQUEST_UNKNOWN_PROCESS_INFO 1
#define OP_PID_DEAD 2
#define OP_REQUEST_EXEC_PROCESS_INFO 3
#define OP_REQUEST_FAULT 4

struct pid_event {
    uint32_t op;
    uint32_t pid;
};

struct pid_event_fault {
    uint32_t op;
    uint32_t pid;
    uint64_t fault_addr;
};


struct pid_event e__;
struct pid_event_fault e___;


struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(u32));
} events SEC(".maps");


static void submit_fault_event(void *ctx, const void *src);

static __always_inline int read_user_faulty(void *ctx, void *dst,  __u32 size, void *src) {
    if (bpf_probe_read_user(dst, size, src) != 0) {
        submit_fault_event(ctx, src);
        return -1;
    }
    return 0;
}

static __always_inline void submit_fault_event(void *ctx, const void *src) {
    struct pid_event_fault e;
    e.op = OP_REQUEST_FAULT;
    e.pid = bpf_get_current_pid_tgid();//todo use namespaced pid
    e.fault_addr = (u64)src;
    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
}

#endif