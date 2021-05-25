#include <sys/types.h>

int phpspy_init(pid_t pid, void* err_ptr, int err_len);
int phpspy_cleanup(pid_t pid, void* err_ptr, int err_len);
int phpspy_snapshot(pid_t pid, void* ptr, int len, void* err_ptr, int err_len);
