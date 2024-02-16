#if !defined(PYROSCOPE_PID)
#define PYROSCOPE_PID

#include "bpf_core_read.h"
#include "bpf_helpers.h"
#include "vmlinux.h"

#define PID_NESTED_NAMESPACES_MAX 4

static __always_inline void current_pid(uint64_t ns_pid_device, uint64_t ns_pid_inode, uint32_t *pid) {
    struct upid upid;
    unsigned int inum;

    if (ns_pid_inode == 0) {
        uint64_t pid_tgid = bpf_get_current_pid_tgid();
        *pid = (u32)(pid_tgid >> 32);
        return;
    }

    struct task_struct *task = (struct task_struct *)bpf_get_current_task();

    // retrieve level nested namespaces
    unsigned int level = BPF_CORE_READ(task, nsproxy, pid_ns_for_children, level);

    // match the level with pid ns inode
#pragma unroll
    for (int i = 0; i < PID_NESTED_NAMESPACES_MAX; i++) {
        if ((level - i) < 0) {
            break;
        }
        upid = BPF_CORE_READ(task, thread_pid, numbers[level - i]);
        inum = BPF_CORE_READ(upid.ns, ns.inum);

        if (inum == ns_pid_inode) {
            *pid = upid.nr;
            break;
        }
    }
}

#endif // PYROSCOPE_PID
