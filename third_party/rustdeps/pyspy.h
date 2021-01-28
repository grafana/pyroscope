#include <sys/types.h>

int pyspy_init(pid_t pid, void* err_ptr, int err_len);
int pyspy_cleanup(pid_t pid, void* err_ptr, int err_len);
int pyspy_snapshot(pid_t pid, void* ptr, int len, void* err_ptr, int err_len);
