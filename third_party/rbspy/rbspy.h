#include <sys/types.h>

int rbspy_init(pid_t pid);
int rbspy_cleanup(pid_t pid);
int rbspy_snapshot(pid_t pid, void* pointer, int len);
