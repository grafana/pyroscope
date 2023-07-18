

#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>

void lib_iter() {
	close(open("file", O_RDWR | O_TRUNC | O_CREAT, 0666));
}