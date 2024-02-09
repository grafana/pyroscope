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
//#define OP_PID_DEAD 2
//#define OP_REQUEST_EXEC_PROCESS_INFO 3

type PidOp uint32

var (
	PidOpRequestUnknownProcessInfo PidOp = 1
	PidOpDead                      PidOp = 2
	PidOpRequestExecProcessInfo    PidOp = 3
)

//#define SAMPLE_KEY_FLAG_PYTHON_STACK 1
//#define SAMPLE_KEY_FLAG_STACK_TRUNCATED 2

type SampleKeyFlag uint32

var (
	SampleKeyFlagPythonStack    SampleKeyFlag = 1
	SampleKeyFlagStackTruncated SampleKeyFlag = 2
)
