
#define PERF_MAX_STACK_DEPTH      127
#define PROFILE_MAPS_SIZE         16384


struct profile_key_t {
	__u32 pid;
	__s64 kern_stack;
	__s64 user_stack;
	char  comm[16];
};

struct profile_bss_args_t {
    __u32 tgid_filter; // 0 => profile everything
};
