

#ifndef PYROEBPF_PTHREAD_BPF_H
#define PYROEBPF_PTHREAD_BPF_H

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "ume.h"
#include "pyoffsets.h"




static inline int pyro_pthread_getspecific(struct libc *libc, int32_t key, void **out) {
    if (key == -1) {
        return -1;
    }
    struct task_struct *task = (struct task_struct *) bpf_get_current_task();
    if (task == NULL) {
        return -1;
    }
    void *tls_base = NULL;

#if defined(__TARGET_ARCH_x86)
    if (pyro_bpf_core_read(&tls_base, sizeof(tls_base), &task->thread.fsbase)) {
        return -1;
    }
#elif defined(__TARGET_ARCH_arm64)
    if (pyro_bpf_core_read(&tls_base, sizeof(tls_base), &task->thread.uw.tp_value)) {
        return -1;
    }
#else
#error "Unknown architecture"
#endif

    if (libc->musl) {
//        0x7fd5a3c0c3be <tss_get>       mov    rax, qword ptr fs:[0]
//        0x7fd5a3c0c3c7 <tss_get+9>     mov    rax, qword ptr [rax + 0x80]
//        0x7fd5a3c0c3ce <tss_get+16>    mov    edi, edi
//        0x7fd5a3c0c3d0 <tss_get+18>    mov    rax, qword ptr [rax + rdi*8]
//        0x7fd5a3c0c3d4 <tss_get+22>    ret
        void *tmp;
        if (bpf_probe_read_user(&tmp,sizeof(tmp), tls_base)) {
            return -1;
        }
        int tsd = libc->musl == 2 ? 0x80 : 0x88;
        if (bpf_probe_read_user(&tmp, sizeof(tmp), tmp + tsd)) {
            return -1;
        }
        if (bpf_probe_read_user(&tmp, sizeof(tmp), tmp + key * 0x8)) {
            return -1;
        }
        *out = tmp;
        return 0;
    }
    void *res;
    // This assumes autoTLSkey < 32, which means that the TLS is stored in
    //   pthread->specific_1stblock[autoTLSkey]
#if defined(__TARGET_ARCH_arm64)
    // # define THREAD_SELF \
    // ((struct pthread *)__builtin_thread_pointer () - 1)
    tls_base -= libc->glibc.pthread_size;
#endif
    if (bpf_probe_read_user(
            &res,
            sizeof(res),
            tls_base + libc->glibc.pthread_specific1stblock + key * 0x10 + 0x08)) {
        return -1;
    }
    *out = res;
    return 0;
}

#endif //PYROEBPF_PTHREAD_BPF_H
