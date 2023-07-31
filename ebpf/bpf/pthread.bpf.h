//
// Created by korniltsev on 31/7/2566.
//

#ifndef PYROEBPF_PTHREAD_BPF_H
#define PYROEBPF_PTHREAD_BPF_H
#include "vmlinux.h"
#include "bpf_helpers.h"

static inline int pyro_pthread_getspecific(uint8_t musl, int32_t key, void *tls_base, void **out) {
    if (musl) {
        return -1; // TODO
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
