//
// Created by korniltsev on 11/21/23.
//

#ifndef PYROEBPF_PTHREAD_ARM64_H
#define PYROEBPF_PTHREAD_ARM64_H

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_core_read.h"
#include "pyoffsets.h"

#if !defined(__TARGET_ARCH_arm64)
#error "Wrong architecture"
#endif

static int pthread_getspecific_musl(const struct libc *libc, int32_t key, void **out, const void *tls_base);
static int pthread_getspecific_glibc(const struct libc *libc, int32_t key, void **out, const void *tls_base);

static __always_inline int pyro_pthread_getspecific(struct libc *libc, int32_t key, void **out) {
    if (key == -1) {
        return -1;
    }
    struct task_struct *task = (struct task_struct *) bpf_get_current_task();
    if (task == NULL) {
        return -1;
    }
    void *tls_base = NULL;
    log_debug("pyro_pthread_getspecific(arm64) key=%d pthread_size=%llx o_pthread_specific1stblock=%llx", key, libc->pthread_size, libc->pthread_specific1stblock);
    if (bpf_core_read(&tls_base, sizeof(tls_base), &task->thread.uw.tp_value)) {
        log_error("pyro_pthread_getspecific(arm64) failed to read task->thread.uw.tp_value");
        return -1;
    }
    log_debug("pyro_pthread_getspecific(arm64)  tls_base=%llx musl=%d", tls_base, libc->musl);


    if (libc->musl) {
        return pthread_getspecific_musl(libc, key, out, tls_base);
    } else {
        return pthread_getspecific_glibc(libc, key, out, tls_base);
    }

    return 0;
}

int __always_inline pthread_getspecific_glibc(const struct libc *libc, int32_t key, void **out, const void *tls_base) {
    void *res = NULL;
    if (key >= 32) {
        return -1; // it is possible to implement this branch, but it's not needed as autoTLSkey is almost always 0
    }
    // This assumes autoTLSkey < 32, which means that the TLS is stored in
    //   pthread->specific_1stblock[autoTLSkey]
    // # define THREAD_SELF \
    // ((struct pthread *)__builtin_thread_pointer () - 1)

    tls_base -= libc->pthread_size;

    if (bpf_probe_read_user(
            &res,
            sizeof(res),
            tls_base + libc->pthread_specific1stblock + key * 0x10 + 0x08)) {
        log_error("pthread_getspecific_glibc(arm64) err 1");
        return -1;
    }
    log_debug("pthread_getspecific_glibc(arm64) res=%llx", res);
    *out = res;
    return 0;
}

int __always_inline pthread_getspecific_musl(const struct libc *libc, int32_t key, void **out, const void *tls_base) {

// example from musl 1.2.4 from alpine 3.18
//        static void *__pthread_getspecific(pthread_key_t k)
//        {
//            struct pthread *self = __pthread_self();
//            return self->tsd[k];
//        }

// #define __pthread_self() ((pthread_t)(__get_tp() - sizeof(struct __pthread) - TP_OFFSET))

//000000000005fc54 <pthread_getspecific>:
//   5fc54:       d53bd041        mrs     x1, tpidr_el0
//   5fc58:       f85a8021        ldur    x1, [x1, #-88]
//   5fc5c:       f8605820        ldr     x0, [x1, w0, uxtw #3]
//   5fc60:       d65f03c0        ret
    void *tmp;
    if (bpf_probe_read_user(&tmp,sizeof(tmp), tls_base - libc->pthread_size + libc->pthread_specific1stblock)) {
        log_error("pthread_getspecific_musl(arm64) err 1");
        return -1;
    }
    if (bpf_probe_read_user(&tmp, sizeof(tmp), tmp + key * 0x8)) {
        log_error("pthread_getspecific_musl(arm64) err 2");
        return -1;
    }
    log_debug("pthread_getspecific_musl(arm64) res=%llx", tmp);
    *out = tmp;
    return 0;
}




#endif //PYROEBPF_PTHREAD_ARM64_H
