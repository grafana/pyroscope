
#if !defined(PYROSCOPE_PID)
#define PYROSCOPE_PID

#define PYROSCOPE_PID_NAMESPACED

#if defined(PYROSCOPE_PID_NAMESPACED)

#include "bpf_core_read.h"
// Good resource on this: https://mozillazg.com/2022/05/ebpf-libbpfgo-get-process-info-en.html
// Using bpf_get_ns_current_pid_tgid is too restrictive for us
//static __always_inline void ns_pid_ppid(struct task_struct *task, u32 *pid , int *ppid, u32 *pid_ns_id) {
static __always_inline void current_pid(struct task_struct *task, u32 *pid) {
    struct upid upid;

    unsigned int level = BPF_CORE_READ(task, nsproxy, pid_ns_for_children, level);
    struct pid *ns_pid = (struct pid *)BPF_CORE_READ(task, group_leader, thread_pid);
    bpf_probe_read_kernel(&upid, sizeof(upid), &ns_pid->numbers[level]);

    *pid = (u32)upid.nr;
//    unsigned int p_level = BPF_CORE_READ(task, real_parent, nsproxy, pid_ns_for_children, level);
//
//    struct pid *ns_ppid = (struct pid *)BPF_CORE_READ(task, real_parent, group_leader, thread_pid);
//    bpf_probe_read_kernel(&upid, sizeof(upid), &ns_ppid->numbers[p_level]);
//    *ppid = upid.nr;
//
//    struct ns_common ns = BPF_CORE_READ(task, nsproxy, pid_ns_for_children, ns);
//    *pid_ns_id = ns.inum;
}

#else // PYROSCOPE_PID_NAMESPACED

static __always_inline void current_pid(struct task_struct *task, u32 *pid) {
  u64 pid_tgid = bpf_get_current_pid_tgid();
  *pid = (u32)(pid_tgid >> 32);
}
#endif // PYROSCOPE_PID_NAMESPACED


#endif // PYROSCOPE_PID