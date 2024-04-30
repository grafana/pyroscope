#ifndef PROFILE_BPF_H
#define PROFILE_BPF_H





#define PROFILING_TYPE_UNKNOWN 1
#define PROFILING_TYPE_FRAMEPOINTERS 2
#define PROFILING_TYPE_PYTHON 3
#define PROFILING_TYPE_ERROR 4

struct pid_config {
    uint8_t type;
    uint8_t collect_user;
    uint8_t collect_kernel;
    uint8_t padding_;
};




struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, u32);
    __type(value, struct pid_config);
    __uint(max_entries, 2048);
} pids SEC(".maps");




struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, 1);
    __type(key, int);
    __array(values, int (void *));
} progs SEC(".maps");

#define PROG_IDX_PYTHON 0

#include "stacks.h"



#endif // PROFILE_BPF_H