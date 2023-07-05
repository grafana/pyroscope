#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>


void lib_iter();

void iter() {
    lib_iter();
}

int main() {
	while (1) {
		iter();
	}
	return 0;

}