#if !defined(PYROSCOPE_PID)
#define PYROSCOPE_PID

static __always_inline void current_pid(uint64_t ns_pid_device, uint64_t ns_pid_inode, uint32_t *pid) {
    // if pid namespace given resolve pids to that namespaces
    if (ns_pid_device && ns_pid_inode) {
        struct bpf_pidns_info ns = {};
        if (bpf_get_ns_current_pid_tgid(ns_pid_device, ns_pid_inode, &ns, sizeof(struct bpf_pidns_info)))
            return;
        *pid = ns.tgid;
    } else {
        uint64_t pid_tgid = bpf_get_current_pid_tgid();
        *pid = (u32)(pid_tgid >> 32);
    }
}

#endif // PYROSCOPE_PID
