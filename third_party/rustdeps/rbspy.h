#include <sys/types.h>

int rbspy_init(pid_t pid, void* err_ptr, int err_len);
int rbspy_cleanup(pid_t pid, void* err_ptr, int err_len);
int rbspy_snapshot(pid_t pid, void* ptr, int len, void* err_ptr, int err_len);
