#if defined(__TARGET_ARCH_x86)
#include "vmlinux-x86.h"
#elif defined(__TARGET_ARCH_arm64)
#include "vmlinux-arm64.h"
#else
#error "Unknown architecture"
#endif