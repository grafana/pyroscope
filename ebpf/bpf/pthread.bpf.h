//
// Created by korniltsev on 31/7/2566.
//

#ifndef PYROEBPF_PTHREAD_BPF_H
#define PYROEBPF_PTHREAD_BPF_H

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "ume.h"

static inline int pyro_get_tlsbase(void **out) {
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
//todo test arm64
    if (pyro_bpf_core_read(&tls_base, sizeof(tls_base), &task->thread.uw.tp_value)) { // todo what is tp_value2??
        return -1;
    }
#else
#error "Unknown architecture"
#endif
    *out = tls_base;
    return 0;
}

// musl 0 means glibc
// musl 1 means musl 1.1.xx
// musl 2 means musl 1.2.xx
static inline int pyro_pthread_getspecific(uint8_t musl, int32_t key, void *tls_base, void **out) {
    if (key == -1) {
        return -1;
    }
    if (musl) {
//        0x7fd5a3c0c3be <tss_get>       mov    rax, qword ptr fs:[0]
//        0x7fd5a3c0c3c7 <tss_get+9>     mov    rax, qword ptr [rax + 0x80]
//        0x7fd5a3c0c3ce <tss_get+16>    mov    edi, edi
//        0x7fd5a3c0c3d0 <tss_get+18>    mov    rax, qword ptr [rax + rdi*8]
//        0x7fd5a3c0c3d4 <tss_get+22>    ret
        void *tmp;
        if (bpf_probe_read_user(&tmp,sizeof(tmp), tls_base)) {
            return -1;
        }
        int tsd = musl == 2 ? 0x80 : 0x88;
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
    // 0x310 is offsetof(struct pthread, specific_1stblock),
    // 0x10 is sizeof(pthread_key_data)
    // 0x8 is offsetof(struct pthread_key_data, data)
    // 'struct pthread' is not in the public API so we have to hardcode
    // the offsets here
    if (bpf_probe_read_user(
            &res,
            sizeof(res),
            tls_base + 0x310 + key * 0x10 + 0x08)) {
        return -1;
    }
    *out = res;
    return 0;
}

#endif //PYROEBPF_PTHREAD_BPF_H
