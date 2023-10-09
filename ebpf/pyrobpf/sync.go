package pyrobpf

type ProfilingType uint8

//#define PROFILING_TYPE_UNKNOWN 1
//#define PROFILING_TYPE_FRAMEPOINTERS 2
//#define PROFILING_TYPE_PYTHON 3
//#define PROFILING_TYPE_ERROR 4

var (
	ProfilingTypeUnknown       ProfilingType = 1
	ProfilingTypeFramepointers ProfilingType = 2
	ProfilingTypePython        ProfilingType = 3
	ProfilingTypeError         ProfilingType = 4
)

//#define OP_REQUEST_UNKNOWN_PROCESS_INFO 1

type PidOp uint32

var (
	PidOpRequestUnknownProcessInfo PidOp = 1
	PidOpDead                      PidOp = 2
)
