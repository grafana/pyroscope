package python

/*
enum {
    STACK_STATUS_COMPLETE = 0,
    STACK_STATUS_ERROR = 1,
    STACK_STATUS_TRUNCATED = 2,
};

enum {
    PY_ERROR_GENERIC = 1,
    PY_ERROR_THREAD_STATE = 2,
    PY_ERROR_THREAD_STATE_NULL = 3,
    PY_ERROR_TOP_FRAME = 4,
    PY_ERROR_FRAME_CODE = 5,
    PY_ERROR_FRAME_PREV = 6,
    PY_ERROR_SYMBOL = 7,
    PY_ERROR_TLSBASE = 8,
    PY_ERROR_FIRST_ARG = 9,
    PY_ERROR_CLASS_NAME = 10,
    PY_ERROR_FILE_NAME = 11,
    PY_ERROR_NAME = 12,



};
*/

type StackStatus uint8

var (
	StackStatusComplete  StackStatus = 0
	StackStatusError     StackStatus = 1
	StackStatusTruncated StackStatus = 2
)

type PyError uint8

var (
	PyErrorGeneric         PyError = 1
	PyErrorThreadState     PyError = 2
	PyErrorThreadStateNull PyError = 3
	PyErrorTopFrame        PyError = 4
	PyErrorFrameCode       PyError = 5
	PyErrorFramePrev       PyError = 6
	PyErrorSymbol          PyError = 7
	PyErrorTlsbase         PyError = 8
	PyErrorFirstArg        PyError = 9
	PyErrorClassName       PyError = 10
	PyErrorFileName        PyError = 11
	PyErrorName            PyError = 12
)
