package symtab

/*
#include <stdlib.h>
static size_t get_malloc__(){
	return (size_t)malloc;
}
*/
import "C"

func testHelperGetMalloc() int {
	return int(C.get_malloc__())
}
