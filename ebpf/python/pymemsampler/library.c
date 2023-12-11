#include <stddef.h>
#include <stdio.h>
#include <stdbool.h>

typedef struct {
    void *ctx;

    void *(*malloc)(void *ctx, size_t size);

    void *(*calloc)(void *ctx, size_t nelem, size_t elsize);

    void *(*realloc)(void *ctx, void *ptr, size_t new_size);

    void (*free)(void *ctx, void *ptr);
} PyMemAllocatorEx;


static bool inc(volatile unsigned long long *counter, unsigned long long value, unsigned long long interval) {
    unsigned long long prev = *counter;
    unsigned long long next = prev + value;
    if (next < interval) {
        *counter = next;
    } else {
        *counter = next % interval;
    }
}


static volatile unsigned long long counter;
const unsigned long long ebpf_assist_interval = 512 * 1024;

PyMemAllocatorEx ebpf_assist_delegate_allocator;




__attribute__((noinline))
static void ebpf_assist_trap(size_t size) {
    asm volatile("\n\t"
    : "=r"(size)
    : "r"(size));
}
const unsigned long long ebpf_assist_trap_ptr = (unsigned long long)ebpf_assist_trap;

__attribute__((constructor))
static void init() {
    printf("libpymemsampler init\n");
}


static void *my_malloc(void *ctx, size_t size) {
    void *res = ebpf_assist_delegate_allocator.malloc(ebpf_assist_delegate_allocator.ctx, size);
    if (res && inc(&counter, size, ebpf_assist_interval)) {
        ebpf_assist_trap(size);
    }
    return res;
}

static void *my_calloc(void *ctx, size_t nelem, size_t elsize) {
    void *res = ebpf_assist_delegate_allocator.calloc(ebpf_assist_delegate_allocator.ctx, nelem, elsize);
    if (res && inc(&counter, nelem * elsize, ebpf_assist_interval)) {
        ebpf_assist_trap(nelem * elsize);
    }
    return res;
}

static void *my_realloc(void *ctx, void *ptr, size_t new_size) {
    void *res = ebpf_assist_delegate_allocator.realloc(ebpf_assist_delegate_allocator.ctx, ptr, new_size);
    if (res && inc(&counter, new_size, ebpf_assist_interval)) {
        ebpf_assist_trap(new_size);
    }
    return res;
}

static void my_free(void *ctx, void *ptr) {
    ebpf_assist_delegate_allocator.free(ebpf_assist_delegate_allocator.ctx, ptr);
}

PyMemAllocatorEx ebpf_assist_sampling_allocator = {
        .ctx = NULL,
        .malloc = my_malloc,
        .calloc = my_calloc,
        .realloc = my_realloc,
        .free = my_free,
};




