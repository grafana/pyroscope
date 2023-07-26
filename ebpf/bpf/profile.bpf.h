#ifndef PROFILE_BPF_H
#define PROFILE_BPF_H

#include "hash.h"

#define PERF_MAX_STACK_DEPTH      127
#define PROFILE_MAPS_SIZE         16384

#define KERN_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP)
#define USER_STACKID_FLAGS (0 | BPF_F_FAST_STACK_CMP | BPF_F_USER_STACK)


struct sample_key {
    __u32 pid;
    __u32 flags;
    __s64 kern_stack;
    __s64 user_stack;
//	char  comm[16];
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

#define HASHED_STRING_LEN 128 // todo can we make it configurable?

typedef struct {
    char str[HASHED_STRING_LEN];
} hashed_string;

typedef uint32_t hashed_string_id;

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, uint32_t);
    __type(value, hashed_string);
    __uint(max_entries, 49152);
} hashed_strings SEC(".maps");


static inline int pyro_hash_string_user(hashed_string *tmp_buf, void *unsafe_ptr, hashed_string_id *out) {
    long sz = bpf_probe_read_user_str(&tmp_buf->str, sizeof(tmp_buf->str), unsafe_ptr);
    if (sz <= 0 || sz > sizeof(tmp_buf->str)) {
        return -1;
    }
    uint32_t str_hash = MurmurHash2(tmp_buf, (int)sz, 0);
    //todo detect hash collision and return error
    if (bpf_map_update_elem(&hashed_strings, &str_hash, tmp_buf, BPF_ANY) != 0) {
        return -2;
    };
    *out = str_hash;
    return 0;
}

#endif // PROFILE_BPF_H