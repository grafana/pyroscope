#ifndef PROFILE_BPF_H
#define PROFILE_BPF_H


#define PERF_MAX_STACK_DEPTH      127
#define PROFILE_MAPS_SIZE         16384

#define KERN_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP)
#define USER_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP | BPF_F_USER_STACK)


struct sample_key {
    __u32 pid;
    __u32 flags;
    __s64 kern_stack;
    __s64 user_stack;
};

struct bss_arg {
//    __u32 tgid_filter; // 0 => profile everything
    __u8 collect_user;
    __u8 collect_kernel;
//    uint32_t num_cpu;
};


struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct sample_key);
    __type(value, u32);
    __uint(max_entries, PROFILE_MAPS_SIZE);
} counts SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_STACK_TRACE);
    __uint(key_size, sizeof(u32));
    __uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
    __uint(max_entries, PROFILE_MAPS_SIZE);
} stacks SEC(".maps");


struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __type(key, u32);
    __type(value, struct bss_arg);
    __uint(max_entries, 1);
} args SEC(".maps");



#endif // PROFILE_BPF_H