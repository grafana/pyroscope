#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>

void iter() {
	close(open("file", O_RDWR | O_TRUNC | O_CREAT, 0666));
}

int main() {
	while (1) {
		iter();
	}
	return 0;

}