//
// Created by korniltsev on 11/21/23.
//

#ifndef PYROEBPF_PTHREAD_AMD64_H
#define PYROEBPF_PTHREAD_AMD64_H

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_core_read.h"
#include "pyoffsets.h"


#if !defined(__TARGET_ARCH_x86)
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

    log_debug("pyro_pthread_getspecific(amd64) key=%d pthread_size=%llx o_pthread_specific1stblock=%llx", key, libc->pthread_size, libc->pthread_specific1stblock);
    if (bpf_core_read(&tls_base, sizeof(tls_base), &task->thread.fsbase)) {
        log_error("pyro_pthread_getspecific(amd64) failed to read fsbase");
        return -1;
    }
    log_debug("pyro_pthread_getspecific(amd64)  tls_base=%llx musl=%d", tls_base, libc->musl);


    if (libc->musl) {
        return pthread_getspecific_musl(libc, key, out, tls_base);

    }
    return pthread_getspecific_glibc(libc, key, out, tls_base);

}

static __always_inline int pthread_getspecific_glibc(const struct libc *libc, int32_t key, void **out, const void *tls_base) {
    void *tmp = NULL;
    if (key >= 32) {
        return -1; // it is possible to implement this branch, but it's not needed as autoTLSkey is almost always 0
    }
    // This assumes autoTLSkey < 32, which means that the TLS is stored in
//   pthread->specific_1stblock[autoTLSkey]
    if (bpf_probe_read_user(
            &tmp,
            sizeof(tmp),
            tls_base + libc->pthread_specific1stblock + key * 0x10 + 0x08)) {
        log_error("pthread_getspecific_glibc(amd64) err 1");
        return -1;
    }
    log_debug("pthread_getspecific_glibc(amd64) res=%llx", tmp);
    *out = tmp;
    return 0;
}

static __always_inline int pthread_getspecific_musl(const struct libc *libc, int32_t key, void **out,
                                    const void *tls_base) {
    // example from musl 1.2.4 from alpine 3.18
//        static void *__pthread_getspecific(pthread_key_t k)
//        {
//            struct pthread *self = __pthread_self();
//            return self->tsd[k];
//        }
//
//        #define __pthread_self() ((pthread_t)__get_tp())
//
//        static inline uintptr_t __get_tp()
//        {
//            uintptr_t tp;
//            __asm__ ("mov %%fs:0,%0" : "=r" (tp) );
//            return tp;
//        }
//
//00000000000563f7 <pthread_getspecific>:
//   563f7:       64 48 8b 04 25 00 00    mov    rax,QWORD PTR fs:0x0
//   563fe:       00 00
//   56400:       48 8b 80 80 00 00 00    mov    rax,QWORD PTR [rax+0x80]  ; << tsd
//   56407:       89 ff                   mov    edi,edi
//   56409:       48 8b 04 f8             mov    rax,QWORD PTR [rax+rdi*8]
//   5640d:       c3                      ret
    void *tmp = NULL;
    if (bpf_probe_read_user(&tmp,sizeof(tmp), tls_base)) {
        log_error("pthread_getspecific_musl(amd64) err 1");
        return -1;
    }
    log_debug("pthread_getspecific_musl(amd64) tmp=%llx", tmp);
    if (bpf_probe_read_user(&tmp, sizeof(tmp), tmp + libc->pthread_specific1stblock)) {
        log_error("pthread_getspecific_musl(amd64) err 2");
        return -1;
    }
    log_debug("pthread_getspecific_musl(amd64) tmp2=%llx", tmp);
    if (bpf_probe_read_user(&tmp, sizeof(tmp), tmp + key * 0x8)) {
        log_error("pthread_getspecific_musl(amd64) err 3");
        return -1;
    }
    log_debug("pthread_getspecific_musl(amd64) res=%llx", tmp);
    *out = tmp;
    return 0;
}

#endif //PYROEBPF_PTHREAD_AMD64_H
