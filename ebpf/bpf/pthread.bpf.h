

#ifndef PYROEBPF_PTHREAD_BPF_H
#define PYROEBPF_PTHREAD_BPF_H




#if defined(__TARGET_ARCH_x86)

#include "pthread_amd64.h"

#elif defined(__TARGET_ARCH_arm64)

#include "pthread_arm64.h"

#else

#error "Unknown architecture"

#endif



#endif //PYROEBPF_PTHREAD_BPF_H
