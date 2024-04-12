// SPDX-License-Identifier: GPL-2.0-only

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "profile.bpf.h"
#include "pid.h"

#define PF_KTHREAD 0x00200000

struct global_config_t {
    uint64_t ns_pid_ino;
};

const volatile struct global_config_t global_config;

SEC("perf_event")
int do_perf_event(struct bpf_perf_event_data *ctx) {
    u32 tgid = 0;
    current_pid(global_config.ns_pid_ino, &tgid);

    struct sample_key key = {};
    u32 *val, one = 1;

    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    if (tgid == 0 || task == 0) {
        return 0;
    }
    int flags = 0;
    if (bpf_core_read(&flags, sizeof(flags), &task->flags)) {
        return 0;
    }
    if (flags & PF_KTHREAD) {
        return 0;
    }

    struct pid_config *config = bpf_map_lookup_elem(&pids, &tgid);
    if (config == NULL) {
        struct pid_config unknown = {
                .type = PROFILING_TYPE_UNKNOWN,
                .collect_kernel = 0,
                .collect_user = 0,
                .padding_ = 0
        };
        if (bpf_map_update_elem(&pids, &tgid, &unknown, BPF_NOEXIST)) {
            return 0;
        }
        struct pid_event event = {
                .op  = OP_REQUEST_UNKNOWN_PROCESS_INFO,
                .pid = tgid
        };
        bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &event, sizeof(event));
        return 0;
    }

    if (config->type == PROFILING_TYPE_ERROR || config->type == PROFILING_TYPE_UNKNOWN) {
        return 0;
    }

    if (config->type == PROFILING_TYPE_PYTHON) {
        bpf_tail_call(ctx, &progs, PROG_IDX_PYTHON);
        return 0;
    }

    if (config->type == PROFILING_TYPE_FRAMEPOINTERS) {
        key.pid = tgid;
        key.kern_stack = -1;
        key.user_stack = -1;

        if (config->collect_kernel) {
            key.kern_stack = bpf_get_stackid(ctx, &stacks, KERN_STACKID_FLAGS);
        }
        if (config->collect_user) {
            key.user_stack = bpf_get_stackid(ctx, &stacks, USER_STACKID_FLAGS);
        }

        val = bpf_map_lookup_elem(&counts, &key);
        if (val)
            (*val)++;
        else
            bpf_map_update_elem(&counts, &key, &one, BPF_NOEXIST);
    }
    return 0;
}


SEC("kprobe/disassociate_ctty")
int BPF_KPROBE(disassociate_ctty, int on_exit) {
    if (!on_exit) {
        return 0;
    }
    u32 pid = 0;
    current_pid(global_config.ns_pid_ino, &pid);
    if (pid == 0) {
        return 0;
    }
    bpf_map_delete_elem(&pids, &pid);
    struct pid_event event = {
        .op  = OP_PID_DEAD,
        .pid = pid
    };
    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}

// execve/execveat
SEC("kprobe/exec")
int BPF_KPROBE(exec, void *_) {
    u32 pid = 0;
    current_pid(global_config.ns_pid_ino, &pid);
    if (pid == 0) {
        return 0;
    }
    struct pid_event event = {
            .op  = OP_REQUEST_EXEC_PROCESS_INFO,
            .pid = pid
    };
    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}

char _license[] SEC("license") = "GPL";
