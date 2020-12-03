#include <sys/types.h>

int pyspy_init(pid_t pid);
int pyspy_cleanup(pid_t pid);
int pyspy_snapshot(pid_t pid, void* pointer, int len);
