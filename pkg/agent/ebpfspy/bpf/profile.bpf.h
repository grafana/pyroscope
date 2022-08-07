
struct profile_key_t {
	__u32 pid;
	__s64 kern_stack;
	__s64 user_stack;
	char  comm[16];
};

struct pid_exit_event {
    __u32 pid;
    __u32 tgid;
    char  comm[16];
};

struct profile_bss_args {
    __u32 tgid_filter; // 0 => profile everything
    __u8  use_tgid_as_key;
    __u8  use_comm;
};