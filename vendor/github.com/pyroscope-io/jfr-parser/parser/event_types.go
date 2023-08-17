package parser

import (
	"fmt"

	"github.com/pyroscope-io/jfr-parser/reader"
)

var events = map[string]func() Parseable{
	"jdk.ActiveRecording":                      func() Parseable { return new(ActiveRecording) },
	"jdk.ActiveSetting":                        func() Parseable { return new(ActiveSetting) },
	"jdk.BooleanFlag":                          func() Parseable { return new(BooleanFlag) },
	"jdk.CPUInformation":                       func() Parseable { return new(CPUInformation) },
	"jdk.CPULoad":                              func() Parseable { return new(CPULoad) },
	"jdk.CPUTimeStampCounter":                  func() Parseable { return new(CPUTimeStampCounter) },
	"jdk.ClassLoaderStatistics":                func() Parseable { return new(ClassLoaderStatistics) },
	"jdk.ClassLoadingStatistics":               func() Parseable { return new(ClassLoadingStatistics) },
	"jdk.CodeCacheConfiguration":               func() Parseable { return new(CodeCacheConfiguration) },
	"jdk.CodeCacheStatistics":                  func() Parseable { return new(CodeCacheStatistics) },
	"jdk.CodeSweeperConfiguration":             func() Parseable { return new(CodeSweeperConfiguration) },
	"jdk.CodeSweeperStatistics":                func() Parseable { return new(CodeSweeperStatistics) },
	"jdk.CompilerConfiguration":                func() Parseable { return new(CompilerConfiguration) },
	"jdk.CompilerStatistics":                   func() Parseable { return new(CompilerStatistics) },
	"jdk.DoubleFlag":                           func() Parseable { return new(DoubleFlag) },
	"jdk.ExceptionStatistics":                  func() Parseable { return new(ExceptionStatistics) },
	"jdk.ExecutionSample":                      func() Parseable { return new(ExecutionSample) },
	"jdk.GCConfiguration":                      func() Parseable { return new(GCConfiguration) },
	"jdk.GCHeapConfiguration":                  func() Parseable { return new(GCHeapConfiguration) },
	"jdk.GCSurvivorConfiguration":              func() Parseable { return new(GCSurvivorConfiguration) },
	"jdk.GCTLABConfiguration":                  func() Parseable { return new(GCTLABConfiguration) },
	"jdk.InitialEnvironmentVariable":           func() Parseable { return new(InitialEnvironmentVariable) },
	"jdk.InitialSystemProperty":                func() Parseable { return new(InitialSystemProperty) },
	"jdk.IntFlag":                              func() Parseable { return new(IntFlag) },
	"jdk.JavaMonitorEnter":                     func() Parseable { return new(JavaMonitorEnter) },
	"jdk.JavaMonitorWait":                      func() Parseable { return new(JavaMonitorWait) },
	"jdk.JavaThreadStatistics":                 func() Parseable { return new(JavaThreadStatistics) },
	"jdk.JVMInformation":                       func() Parseable { return new(JVMInformation) },
	"jdk.LoaderConstraintsTableStatistics":     func() Parseable { return new(LoaderConstraintsTableStatistics) },
	"jdk.LongFlag":                             func() Parseable { return new(LongFlag) },
	"jdk.ModuleExport":                         func() Parseable { return new(ModuleExport) },
	"jdk.ModuleRequire":                        func() Parseable { return new(ModuleRequire) },
	"jdk.NativeLibrary":                        func() Parseable { return new(NativeLibrary) },
	"jdk.NetworkUtilization":                   func() Parseable { return new(NetworkUtilization) },
	"jdk.ObjectAllocationInNewTLAB":            func() Parseable { return new(ObjectAllocationInNewTLAB) },
	"jdk.ObjectAllocationOutsideTLAB":          func() Parseable { return new(ObjectAllocationOutsideTLAB) },
	"jdk.OSInformation":                        func() Parseable { return new(OSInformation) },
	"jdk.PhysicalMemory":                       func() Parseable { return new(PhysicalMemory) },
	"jdk.PlaceholderTableStatistics":           func() Parseable { return new(PlaceholderTableStatistics) },
	"jdk.ProtectionDomainCacheTableStatistics": func() Parseable { return new(ProtectionDomainCacheTableStatistics) },
	"jdk.StringFlag":                           func() Parseable { return new(StringFlag) },
	"jdk.StringTableStatistics":                func() Parseable { return new(StringTableStatistics) },
	"jdk.SymbolTableStatistics":                func() Parseable { return new(SymbolTableStatistics) },
	"jdk.SystemProcess":                        func() Parseable { return new(SystemProcess) },
	"jdk.ThreadAllocationStatistics":           func() Parseable { return new(ThreadAllocationStatistics) },
	"jdk.ThreadCPULoad":                        func() Parseable { return new(ThreadCPULoad) },
	"jdk.ThreadContextSwitchRate":              func() Parseable { return new(ThreadContextSwitchRate) },
	"jdk.ThreadDump":                           func() Parseable { return new(ThreadDump) },
	"jdk.ThreadPark":                           func() Parseable { return new(ThreadPark) },
	"jdk.ThreadStart":                          func() Parseable { return new(ThreadStart) },
	"jdk.UnsignedIntFlag":                      func() Parseable { return new(UnsignedIntFlag) },
	"jdk.UnsignedLongFlag":                     func() Parseable { return new(UnsignedLongFlag) },
	"jdk.VirtualizationInformation":            func() Parseable { return new(VirtualizationInformation) },
	"jdk.YoungGenerationConfiguration":         func() Parseable { return new(YoungGenerationConfiguration) },
	"profiler.LiveObject":                      func() Parseable { return new(LiveObject) },
}

func ParseEvent(r reader.Reader, classes ClassMap, cpools PoolMap) (Parseable, error) {
	kind, err := r.VarLong()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve event type: %w", err)
	}
	return parseEvent(r, classes, cpools, int(kind))
}

func parseEvent(r reader.Reader, classes ClassMap, cpools PoolMap, classID int) (Parseable, error) {
	class, ok := classes[classID]
	if !ok {
		return nil, fmt.Errorf("unknown class %d", classID)
	}
	v := class.eventFn()
	if err := v.Parse(r, classes, cpools, class); err != nil {
		return nil, fmt.Errorf("unable to parse event %s: %w", class.Name, err)
	}
	return v, nil
}

type ActiveRecording struct {
	StartTime         int64
	Duration          int64
	EventThread       *Thread
	ID                int64
	Name              string
	Destination       string
	MaxAge            int64
	MaxSize           int64
	RecordingStart    int64
	RecordingDuration int64
}

func (ar *ActiveRecording) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ar.StartTime, err = toLong(r)
	case "duration":
		ar.Duration, err = toLong(r)
	case "eventThread":
		ar.EventThread, err = toThread(p)
	case "id":
		ar.ID, err = toLong(r)
	case "name":
		ar.Name, err = toString(r)
	case "destination":
		ar.Destination, err = toString(r)
	case "maxAge":
		ar.MaxAge, err = toLong(r)
	case "maxSize":
		ar.MaxSize, err = toLong(r)
	case "recordingStart":
		ar.RecordingStart, err = toLong(r)
	case "recordingDuration":
		ar.RecordingDuration, err = toLong(r)
	}
	return err
}

func (ar *ActiveRecording) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ar.parseField)
}

type ActiveSetting struct {
	StartTime   int64
	Duration    int64
	EventThread *Thread
	ID          int64
	Name        string
	Value       string
}

func (as *ActiveSetting) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		as.StartTime, err = toLong(r)
	case "duration":
		as.Duration, err = toLong(r)
	case "eventThread":
		as.EventThread, err = toThread(p)
	case "id":
		as.ID, err = toLong(r)
	case "name":
		as.Name, err = toString(r)
	case "value":
		as.Value, err = toString(r)
	}
	return err
}

func (as *ActiveSetting) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, as.parseField)
}

type BooleanFlag struct {
	StartTime int64
	Name      string
	Value     bool
	Origin    *FlagValueOrigin
}

func (bf *BooleanFlag) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		bf.StartTime, err = toLong(r)
	case "name":
		bf.Name, err = toString(r)
	case "value":
		bf.Value, err = toBoolean(r)
	case "origin":
		bf.Origin, err = toFlagValueOrigin(p)
	}
	return err
}

func (bf *BooleanFlag) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, bf.parseField)
}

type CPUInformation struct {
	StartTime   int64
	CPU         string
	Description string
	Sockets     int32
	Cores       int32
	HWThreads   int32
}

func (ci *CPUInformation) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ci.StartTime, err = toLong(r)
	case "duration":
		ci.CPU, err = toString(r)
	case "eventThread":
		ci.Description, err = toString(r)
	case "sockets":
		ci.Sockets, err = toInt(r)
	case "cores":
		ci.Cores, err = toInt(r)
	case "hwThreads":
		ci.HWThreads, err = toInt(r)
	}
	return err
}

func (ci *CPUInformation) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ci.parseField)
}

type CPULoad struct {
	StartTime    int64
	JVMUser      float32
	JVMSystem    float32
	MachineTotal float32
}

func (cl *CPULoad) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		cl.StartTime, err = toLong(r)
	case "jvmUser":
		cl.JVMUser, err = toFloat(r)
	case "jvmSystem":
		cl.JVMSystem, err = toFloat(r)
	case "machineTotal":
		cl.MachineTotal, err = toFloat(r)
	}
	return err
}

func (cl *CPULoad) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, cl.parseField)
}

type CPUTimeStampCounter struct {
	StartTime           int64
	FastTimeEnabled     bool
	FastTimeAutoEnabled bool
	OSFrequency         int64
	FastTimeFrequency   int64
}

func (ctsc *CPUTimeStampCounter) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ctsc.StartTime, err = toLong(r)
	case "fastTimeEnabled":
		ctsc.FastTimeEnabled, err = toBoolean(r)
	case "fastTimeAutoEnabled":
		ctsc.FastTimeAutoEnabled, err = toBoolean(r)
	case "osFrequency":
		ctsc.OSFrequency, err = toLong(r)
	case "fastTimeFrequency":
		ctsc.FastTimeFrequency, err = toLong(r)
	}
	return err
}

func (ctsc *CPUTimeStampCounter) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ctsc.parseField)
}

type ClassLoaderStatistics struct {
	StartTime                 int64
	ClassLoader               *ClassLoader
	ParentClassLoader         *ClassLoader
	ClassLoaderData           int64
	ClassCount                int64
	ChunkSize                 int64
	BlockSize                 int64
	AnonymousClassCount       int64
	AnonymousChunkSize        int64
	AnonymousBlockSize        int64
	UnsafeAnonymousClassCount int64
	UnsafeAnonymousChunkSize  int64
	UnsafeAnonymousBlockSize  int64
	HiddenClassCount          int64
	HiddenChunkSize           int64
	HiddenBlockSize           int64
}

func (cls *ClassLoaderStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		cls.StartTime, err = toLong(r)
	case "classLoader":
		cls.ClassLoader, err = toClassLoader(p)
	case "parentClassLoader":
		cls.ParentClassLoader, err = toClassLoader(p)
	case "classLoaderData":
		cls.ClassLoaderData, err = toLong(r)
	case "classCount":
		cls.ClassCount, err = toLong(r)
	case "chunkSize":
		cls.ChunkSize, err = toLong(r)
	case "blockSize":
		cls.BlockSize, err = toLong(r)
	case "anonymousClassCount":
		cls.AnonymousClassCount, err = toLong(r)
	case "anonymousChunkSize":
		cls.AnonymousChunkSize, err = toLong(r)
	case "anonymousBlockSize":
		cls.AnonymousBlockSize, err = toLong(r)
	case "unsafeAnonymousClassCount":
		cls.UnsafeAnonymousClassCount, err = toLong(r)
	case "unsafeAnonymousChunkSize":
		cls.UnsafeAnonymousChunkSize, err = toLong(r)
	case "unsafeAnonymousBlockSize":
		cls.UnsafeAnonymousBlockSize, err = toLong(r)
	case "hiddenClassCount":
		cls.HiddenClassCount, err = toLong(r)
	case "hiddenChunkSize":
		cls.HiddenChunkSize, err = toLong(r)
	case "hiddenBlockSize":
		cls.HiddenBlockSize, err = toLong(r)
	}
	return err
}

func (cls *ClassLoaderStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, cls.parseField)
}

type ClassLoadingStatistics struct {
	StartTime          int64
	LoadedClassCount   int64
	UnloadedClassCount int64
}

func (cls *ClassLoadingStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		cls.StartTime, err = toLong(r)
	case "loadedClassCount":
		cls.LoadedClassCount, err = toLong(r)
	case "unloadedClassCount":
		cls.UnloadedClassCount, err = toLong(r)
	}
	return err
}

func (cls *ClassLoadingStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, cls.parseField)
}

type CodeCacheConfiguration struct {
	StartTime          int64
	InitialSize        int64
	ReservedSize       int64
	NonNMethodSize     int64
	ProfiledSize       int64
	NonProfiledSize    int64
	ExpansionSize      int64
	MinBlockLength     int64
	StartAddress       int64
	ReservedTopAddress int64
}

func (ccc *CodeCacheConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ccc.StartTime, err = toLong(r)
	case "initialSize":
		ccc.InitialSize, err = toLong(r)
	case "reservedSize":
		ccc.ReservedSize, err = toLong(r)
	case "nonNMethodSize":
		ccc.NonNMethodSize, err = toLong(r)
	case "profiledSize":
		ccc.ProfiledSize, err = toLong(r)
	case "NonProfiledSize":
		ccc.NonProfiledSize, err = toLong(r)
	case "ExpansionSize":
		ccc.ExpansionSize, err = toLong(r)
	case "MinBlockLength":
		ccc.MinBlockLength, err = toLong(r)
	case "StartAddress":
		ccc.StartAddress, err = toLong(r)
	case "ReservedTopAddress":
		ccc.ReservedTopAddress, err = toLong(r)
	}
	return err
}

func (ccc *CodeCacheConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ccc.parseField)
}

type CodeCacheStatistics struct {
	StartTime           int64
	CodeBlobType        *CodeBlobType
	StartAddress        int64
	ReservedTopAddress  int64
	EntryCount          int32
	MethodCount         int32
	AdaptorCount        int32
	UnallocatedCapacity int64
	FullCount           int32
}

func (ccs *CodeCacheStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ccs.StartTime, err = toLong(r)
	case "codeBlobType":
		ccs.CodeBlobType, err = toCodeBlobType(p)
	case "startAddress":
		ccs.StartAddress, err = toLong(r)
	case "reservedTopAddress":
		ccs.ReservedTopAddress, err = toLong(r)
	case "entryCount":
		ccs.EntryCount, err = toInt(r)
	case "methodCount":
		ccs.MethodCount, err = toInt(r)
	case "adaptorCount":
		ccs.AdaptorCount, err = toInt(r)
	case "unallocatedCapacity":
		ccs.UnallocatedCapacity, err = toLong(r)
	case "fullCount":
		ccs.FullCount, err = toInt(r)
	}
	return err
}

func (ccs *CodeCacheStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ccs.parseField)
}

type CodeSweeperConfiguration struct {
	StartTime       int64
	SweeperEnabled  bool
	FlushingEnabled bool
	SweepThreshold  int64
}

func (csc *CodeSweeperConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		csc.StartTime, err = toLong(r)
	case "sweeperEnabled":
		csc.SweeperEnabled, err = toBoolean(r)
	case "flushingEnabled":
		csc.FlushingEnabled, err = toBoolean(r)
	case "sweepThreshold":
		csc.SweepThreshold, err = toLong(r)
	}
	return err
}

func (csc *CodeSweeperConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, csc.parseField)
}

type CodeSweeperStatistics struct {
	StartTime            int64
	SweepCount           int32
	MethodReclaimedCount int32
	TotalSweepTime       int64
	PeakFractionTime     int64
	PeakSweepTime        int64
}

func (css *CodeSweeperStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		css.StartTime, err = toLong(r)
	case "sweepCount":
		css.SweepCount, err = toInt(r)
	case "methodReclaimedCount":
		css.MethodReclaimedCount, err = toInt(r)
	case "totalSweepTime":
		css.TotalSweepTime, err = toLong(r)
	case "peakFractionTime":
		css.PeakFractionTime, err = toLong(r)
	case "peakSweepTime":
		css.PeakSweepTime, err = toLong(r)
	}
	return err
}

func (css *CodeSweeperStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, css.parseField)
}

type CompilerConfiguration struct {
	StartTime         int64
	ThreadCount       int32
	TieredCompilation bool
}

func (cc *CompilerConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		cc.StartTime, err = toLong(r)
	case "threadCount":
		cc.ThreadCount, err = toInt(r)
	case "tieredCompilation":
		cc.TieredCompilation, err = toBoolean(r)
	}
	return err
}

func (cc *CompilerConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, cc.parseField)
}

type CompilerStatistics struct {
	StartTime             int64
	CompileCount          int32
	BailoutCount          int32
	InvalidatedCount      int32
	OSRCompileCount       int32
	StandardCompileCount  int32
	OSRBytesCompiled      int64
	StandardBytesCompiled int64
	NMethodsSize          int64
	NMethodCodeSize       int64
	PeakTimeSpent         int64
	TotalTimeSpent        int64
}

func (cs *CompilerStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		cs.StartTime, err = toLong(r)
	case "compileCount":
		cs.CompileCount, err = toInt(r)
	case "bailoutCount":
		cs.BailoutCount, err = toInt(r)
	case "invalidatedCount":
		cs.InvalidatedCount, err = toInt(r)
	case "osrCompileCount":
		cs.OSRCompileCount, err = toInt(r)
	case "standardCompileCount":
		cs.StandardCompileCount, err = toInt(r)
	case "osrBytesCompiled":
		cs.OSRBytesCompiled, err = toLong(r)
	case "standardBytesCompiled":
		cs.StandardBytesCompiled, err = toLong(r)
	case "nmethodsSize":
		cs.NMethodsSize, err = toLong(r)
	case "nmethodCodeSize":
		cs.NMethodCodeSize, err = toLong(r)
	case "peakTimeSpent":
		cs.PeakTimeSpent, err = toLong(r)
	case "totalTimeSpent":
		cs.TotalTimeSpent, err = toLong(r)
	}
	return err
}

func (cs *CompilerStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, cs.parseField)
}

type DoubleFlag struct {
	StartTime int64
	Name      string
	Value     float64
	Origin    *FlagValueOrigin
}

func (df *DoubleFlag) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		df.StartTime, err = toLong(r)
	case "name":
		df.Name, err = toString(r)
	case "value":
		df.Value, err = toDouble(r)
	case "origin":
		df.Origin, err = toFlagValueOrigin(p)
	}
	return err
}

func (df *DoubleFlag) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, df.parseField)
}

type ExceptionStatistics struct {
	StartTime   int64
	Duration    int64
	EventThread *Thread
	StackTrace  *StackTrace
	Throwable   int64
}

func (es *ExceptionStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		es.StartTime, err = toLong(r)
	case "duration":
		es.Duration, err = toLong(r)
	case "eventThread":
		es.EventThread, err = toThread(p)
	case "stackTrace":
		es.StackTrace, err = toStackTrace(p)
	case "throwable":
		es.Throwable, err = toLong(r)
	}
	return err
}

func (es *ExceptionStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, es.parseField)
}

type ExecutionSample struct {
	StartTime     int64
	SampledThread *Thread
	StackTrace    *StackTrace
	State         *ThreadState
	ContextId     int64
}

func (es *ExecutionSample) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		es.StartTime, err = toLong(r)
	case "sampledThread":
		es.SampledThread, err = toThread(p)
	case "stackTrace":
		es.StackTrace, err = toStackTrace(p)
	case "state":
		es.State, err = toThreadState(p)
	case "contextId":
		es.ContextId, err = toLong(r)
	}
	return err
}

func (es *ExecutionSample) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, es.parseField)
}

type GCConfiguration struct {
	StartTime              int64
	YoungCollector         *GCName
	OldCollector           *GCName
	ParallelGCThreads      int32
	ConcurrentGCThreads    int32
	UsesDynamicGCThreads   bool
	IsExplicitGCConcurrent bool
	IsExplicitGCDisabled   bool
	PauseTarget            int64
	GCTimeRatio            int32
}

func (gc *GCConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		gc.StartTime, err = toLong(r)
	case "youngCollector":
		gc.YoungCollector, err = toGCName(p)
	case "oldCollector":
		gc.OldCollector, err = toGCName(p)
	case "parallelGCThreads":
		gc.ParallelGCThreads, err = toInt(r)
	case "concurrentGCThreads":
		gc.ConcurrentGCThreads, err = toInt(r)
	case "usesDynamicGCThreads":
		gc.UsesDynamicGCThreads, err = toBoolean(r)
	case "isExplicitGCConcurrent":
		gc.IsExplicitGCConcurrent, err = toBoolean(r)
	case "isExplicitGCDisabled":
		gc.IsExplicitGCDisabled, err = toBoolean(r)
	case "pauseTarget":
		gc.PauseTarget, err = toLong(r)
	case "gcTimeRatio":
		gc.GCTimeRatio, err = toInt(r)
	}
	return err
}

func (gc *GCConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, gc.parseField)
}

type GCHeapConfiguration struct {
	StartTime          int64
	MinSize            int64
	MaxSize            int64
	InitialSize        int64
	UsesCompressedOops bool
	CompressedOopsMode *NarrowOopMode
	ObjectAlignment    int64
	HeapAddressBits    int8
}

func (ghc *GCHeapConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ghc.StartTime, err = toLong(r)
	case "minSize":
		ghc.MinSize, err = toLong(r)
	case "maxSize":
		ghc.MaxSize, err = toLong(r)
	case "initialSize":
		ghc.InitialSize, err = toLong(r)
	case "usesCompressedOops":
		ghc.UsesCompressedOops, err = toBoolean(r)
	case "compressedOopsMode":
		ghc.CompressedOopsMode, err = toNarrowOopMode(p)
	case "objectAlignment":
		ghc.ObjectAlignment, err = toLong(r)
	case "heapAddressBits":
		ghc.HeapAddressBits, err = toByte(r)
	}
	return err
}

func (ghc *GCHeapConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ghc.parseField)
}

type GCSurvivorConfiguration struct {
	StartTime                int64
	MaxTenuringThreshold     int8
	InitialTenuringThreshold int8
}

func (gcs *GCSurvivorConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		gcs.StartTime, err = toLong(r)
	case "maxTenuringThreshold":
		gcs.MaxTenuringThreshold, err = toByte(r)
	case "initialTenuringThreshold":
		gcs.InitialTenuringThreshold, err = toByte(r)
	}
	return err
}

func (gsc *GCSurvivorConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, gsc.parseField)
}

type GCTLABConfiguration struct {
	StartTime            int64
	UsesTLABs            bool
	MinTLABSize          int64
	TLABRefillWasteLimit int64
}

func (gtc *GCTLABConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		gtc.StartTime, err = toLong(r)
	case "usesTLABs":
		gtc.UsesTLABs, err = toBoolean(r)
	case "minTLABSize":
		gtc.MinTLABSize, err = toLong(r)
	case "tlabRefillWasteLimit":
		gtc.TLABRefillWasteLimit, err = toLong(r)
	}
	return err
}

func (gtc *GCTLABConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, gtc.parseField)
}

type InitialEnvironmentVariable struct {
	StartTime int64
	Key       string
	Value     string
}

func (iev *InitialEnvironmentVariable) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		iev.StartTime, err = toLong(r)
	case "key":
		iev.Key, err = toString(r)
	case "value":
		iev.Value, err = toString(r)
	}
	return err
}

func (iev *InitialEnvironmentVariable) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, iev.parseField)
}

type InitialSystemProperty struct {
	StartTime int64
	Key       string
	Value     string
}

func (isp *InitialSystemProperty) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		isp.StartTime, err = toLong(r)
	case "key":
		isp.Key, err = toString(r)
	case "value":
		isp.Value, err = toString(r)
	}
	return err
}

func (isp *InitialSystemProperty) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, isp.parseField)
}

type IntFlag struct {
	StartTime int64
	Name      string
	Value     int32
	Origin    *FlagValueOrigin
}

func (f *IntFlag) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		f.StartTime, err = toLong(r)
	case "name":
		f.Name, err = toString(r)
	case "value":
		f.Value, err = toInt(r)
	case "origin":
		f.Origin, err = toFlagValueOrigin(p)
	}
	return err
}

func (f *IntFlag) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, f.parseField)
}

type JavaMonitorEnter struct {
	StartTime     int64
	Duration      int64
	EventThread   *Thread
	StackTrace    *StackTrace
	MonitorClass  *Class
	PreviousOwner *Thread
	Address       int64
	ContextId     int64
}

func (jme *JavaMonitorEnter) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		jme.StartTime, err = toLong(r)
	case "duration":
		jme.Duration, err = toLong(r)
	case "eventThread":
		jme.EventThread, err = toThread(p)
	case "stackTrace":
		jme.StackTrace, err = toStackTrace(p)
	case "monitorClass":
		jme.MonitorClass, err = toClass(p)
	case "previousOwner":
		jme.PreviousOwner, err = toThread(p)
	case "address":
		jme.Address, err = toLong(r)
	case "contextId":
		jme.ContextId, err = toLong(r)
	}
	return err
}

func (jme *JavaMonitorEnter) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, jme.parseField)
}

type JavaMonitorWait struct {
	StartTime    int64
	Duration     int64
	EventThread  *Thread
	StackTrace   *StackTrace
	MonitorClass *Class
	Notifier     *Thread
	Timeout      int64
	TimedOut     bool
	Address      int64
}

func (jmw *JavaMonitorWait) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		jmw.StartTime, err = toLong(r)
	case "duration":
		jmw.Duration, err = toLong(r)
	case "eventThread":
		jmw.EventThread, err = toThread(p)
	case "stackTrace":
		jmw.StackTrace, err = toStackTrace(p)
	case "monitorClass":
		jmw.MonitorClass, err = toClass(p)
	case "notifier":
		jmw.Notifier, err = toThread(p)
	case "timeout":
		jmw.Timeout, err = toLong(r)
	case "timedOut":
		jmw.TimedOut, err = toBoolean(r)
	case "address":
		jmw.Address, err = toLong(r)
	}
	return err
}

func (jmw *JavaMonitorWait) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, jmw.parseField)
}

type JavaThreadStatistics struct {
	StartTime        int64
	ActiveCount      int64
	DaemonCount      int64
	AccumulatedCount int64
	PeakCount        int64
}

func (jts *JavaThreadStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		jts.StartTime, err = toLong(r)
	case "activeCount":
		jts.ActiveCount, err = toLong(r)
	case "daemonCount":
		jts.DaemonCount, err = toLong(r)
	case "accumulatedCount":
		jts.AccumulatedCount, err = toLong(r)
	case "peakCount":
		jts.PeakCount, err = toLong(r)
	}
	return err
}

func (jts *JavaThreadStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, jts.parseField)
}

type JVMInformation struct {
	StartTime     int64
	JVMName       string
	JVMVersion    string
	JVMArguments  string
	JVMFlags      string
	JavaArguments string
	JVMStartTime  int64
	PID           int64
}

func (ji *JVMInformation) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ji.StartTime, err = toLong(r)
	case "jvmName":
		ji.JVMName, err = toString(r)
	case "jvmVersion":
		ji.JVMVersion, err = toString(r)
	case "jvmArguments":
		ji.JVMArguments, err = toString(r)
	case "jvmFlags":
		ji.JVMFlags, err = toString(r)
	case "javaArguments":
		ji.JavaArguments, err = toString(r)
	case "jvmStartTime":
		ji.JVMStartTime, err = toLong(r)
	case "pid":
		ji.PID, err = toLong(r)
	}
	return err
}

func (ji *JVMInformation) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ji.parseField)
}

type LoaderConstraintsTableStatistics struct {
	StartTime                    int64
	BucketCount                  int64
	EntryCount                   int64
	TotalFootprint               int64
	BucketCountMaximum           int64
	BucketCountAverage           float32
	BucketCountVariance          float32
	BucketCountStandardDeviation float32
	InsertionRate                float32
	RemovalRate                  float32
}

func (lcts *LoaderConstraintsTableStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		lcts.StartTime, err = toLong(r)
	case "bucketCount":
		lcts.BucketCount, err = toLong(r)
	case "entryCount":
		lcts.EntryCount, err = toLong(r)
	case "totalFootprint":
		lcts.TotalFootprint, err = toLong(r)
	case "bucketCountMaximum":
		lcts.BucketCountMaximum, err = toLong(r)
	case "bucketCountAverage":
		lcts.BucketCountAverage, err = toFloat(r)
	case "bucketCountVariance":
		lcts.BucketCountVariance, err = toFloat(r)
	case "bucketCountStandardDeviation":
		lcts.BucketCountStandardDeviation, err = toFloat(r)
	case "insertionRate":
		lcts.InsertionRate, err = toFloat(r)
	case "removalRate":
		lcts.RemovalRate, err = toFloat(r)
	}
	return err
}

func (lcts *LoaderConstraintsTableStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, lcts.parseField)
}

type LongFlag struct {
	StartTime int64
	Name      string
	Value     int64
	Origin    *FlagValueOrigin
}

func (lf *LongFlag) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		lf.StartTime, err = toLong(r)
	case "name":
		lf.Name, err = toString(r)
	case "value":
		lf.Value, err = toLong(r)
	case "origin":
		lf.Origin, err = toFlagValueOrigin(p)
	}
	return err
}

func (lf *LongFlag) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, lf.parseField)
}

type ModuleExport struct {
	StartTime       int64
	ExportedPackage *Package
	TargetModule    *Module
}

func (me *ModuleExport) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		me.StartTime, err = toLong(r)
	case "exportedPackage":
		me.ExportedPackage, err = toPackage(p)
	case "targetModule":
		me.TargetModule, err = toModule(p)
	}
	return err
}

func (me *ModuleExport) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, me.parseField)
}

type ModuleRequire struct {
	StartTime      int64
	Source         *Module
	RequiredModule *Module
}

func (mr *ModuleRequire) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		mr.StartTime, err = toLong(r)
	case "sourced":
		mr.Source, err = toModule(p)
	case "requiredModule":
		mr.RequiredModule, err = toModule(p)
	}
	return err
}

func (mr *ModuleRequire) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, mr.parseField)
}

type NativeLibrary struct {
	StartTime   int64
	Name        string
	BaseAddress int64
	TopAddress  int64
}

func (nl *NativeLibrary) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		nl.StartTime, err = toLong(r)
	case "name":
		nl.Name, err = toString(r)
	case "baseAddress":
		nl.BaseAddress, err = toLong(r)
	case "topAddress":
		nl.TopAddress, err = toLong(r)
	}
	return err
}

func (nl *NativeLibrary) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, nl.parseField)
}

type NetworkUtilization struct {
	StartTime        int64
	NetworkInterface *NetworkInterfaceName
	ReadRate         int64
	WriteRate        int64
}

func (nu *NetworkUtilization) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		nu.StartTime, err = toLong(r)
	case "networkInterface":
		nu.NetworkInterface, err = toNetworkInterfaceName(p)
	case "readRate":
		nu.ReadRate, err = toLong(r)
	case "writeRate":
		nu.WriteRate, err = toLong(r)
	}
	return err
}

func (nu *NetworkUtilization) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, nu.parseField)
}

type ObjectAllocationInNewTLAB struct {
	StartTime      int64
	EventThread    *Thread
	StackTrace     *StackTrace
	ObjectClass    *Class
	AllocationSize int64
	TLABSize       int64
	ContextId      int64
}

func (oa *ObjectAllocationInNewTLAB) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		oa.StartTime, err = toLong(r)
	case "eventThread":
		oa.EventThread, err = toThread(p)
	case "stackTrace":
		oa.StackTrace, err = toStackTrace(p)
	case "objectClass":
		oa.ObjectClass, err = toClass(p)
	case "allocationSize":
		oa.AllocationSize, err = toLong(r)
	case "tlabSize":
		oa.TLABSize, err = toLong(r)
	case "contextId":
		oa.ContextId, err = toLong(r)
	}

	return err
}

func (oa *ObjectAllocationInNewTLAB) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, oa.parseField)
}

type ObjectAllocationOutsideTLAB struct {
	StartTime      int64
	EventThread    *Thread
	StackTrace     *StackTrace
	ObjectClass    *Class
	AllocationSize int64
	ContextId      int64
}

func (oa *ObjectAllocationOutsideTLAB) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		oa.StartTime, err = toLong(r)
	case "eventThread":
		oa.EventThread, err = toThread(p)
	case "stackTrace":
		oa.StackTrace, err = toStackTrace(p)
	case "objectClass":
		oa.ObjectClass, err = toClass(p)
	case "allocationSize":
		oa.AllocationSize, err = toLong(r)
	case "contextId":
		oa.ContextId, err = toLong(r)
	}
	return err
}

func (oa *ObjectAllocationOutsideTLAB) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, oa.parseField)
}

type OSInformation struct {
	StartTime int64
	OSVersion string
}

func (os *OSInformation) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		os.StartTime, err = toLong(r)
	case "osVersion":
		os.OSVersion, err = toString(r)
	}
	return err
}

func (os *OSInformation) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, os.parseField)
}

type PhysicalMemory struct {
	StartTime int64
	TotalSize int64
	UsedSize  int64
}

func (pm *PhysicalMemory) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		pm.StartTime, err = toLong(r)
	case "totalSize":
		pm.TotalSize, err = toLong(r)
	case "usedSize":
		pm.UsedSize, err = toLong(r)
	}
	return err
}

func (pm *PhysicalMemory) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, pm.parseField)
}

type PlaceholderTableStatistics struct {
	StartTime                    int64
	BucketCount                  int64
	EntryCount                   int64
	TotalFootprint               int64
	BucketCountMaximum           int64
	BucketCountAverage           float32
	BucketCountVariance          float32
	BucketCountStandardDeviation float32
	InsertionRate                float32
	RemovalRate                  float32
}

func (pts *PlaceholderTableStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		pts.StartTime, err = toLong(r)
	case "bucketCount":
		pts.BucketCount, err = toLong(r)
	case "entryCount":
		pts.EntryCount, err = toLong(r)
	case "totalFootprint":
		pts.TotalFootprint, err = toLong(r)
	case "bucketCountMaximum":
		pts.BucketCountMaximum, err = toLong(r)
	case "bucketCountAverage":
		pts.BucketCountAverage, err = toFloat(r)
	case "bucketCountVariance":
		pts.BucketCountVariance, err = toFloat(r)
	case "bucketCountStandardDeviation":
		pts.BucketCountStandardDeviation, err = toFloat(r)
	case "insertionRate":
		pts.InsertionRate, err = toFloat(r)
	case "removalRate":
		pts.RemovalRate, err = toFloat(r)
	}
	return err
}

func (pts *PlaceholderTableStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, pts.parseField)
}

type ProtectionDomainCacheTableStatistics struct {
	StartTime                    int64
	BucketCount                  int64
	EntryCount                   int64
	TotalFootprint               int64
	BucketCountMaximum           int64
	BucketCountAverage           float32
	BucketCountVariance          float32
	BucketCountStandardDeviation float32
	InsertionRate                float32
	RemovalRate                  float32
}

func (pdcts *ProtectionDomainCacheTableStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		pdcts.StartTime, err = toLong(r)
	case "bucketCount":
		pdcts.BucketCount, err = toLong(r)
	case "entryCount":
		pdcts.EntryCount, err = toLong(r)
	case "totalFootprint":
		pdcts.TotalFootprint, err = toLong(r)
	case "bucketCountMaximum":
		pdcts.BucketCountMaximum, err = toLong(r)
	case "bucketCountAverage":
		pdcts.BucketCountAverage, err = toFloat(r)
	case "bucketCountVariance":
		pdcts.BucketCountVariance, err = toFloat(r)
	case "bucketCountStandardDeviation":
		pdcts.BucketCountStandardDeviation, err = toFloat(r)
	case "insertionRate":
		pdcts.InsertionRate, err = toFloat(r)
	case "removalRate":
		pdcts.RemovalRate, err = toFloat(r)
	}
	return err
}

func (pdcts *ProtectionDomainCacheTableStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, pdcts.parseField)
}

type StringFlag struct {
	StartTime int64
	Name      string
	Value     string
	Origin    *FlagValueOrigin
}

func (sf *StringFlag) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		sf.StartTime, err = toLong(r)
	case "name":
		sf.Name, err = toString(r)
	case "value":
		sf.Value, err = toString(r)
	case "origin":
		sf.Origin, err = toFlagValueOrigin(p)
	}
	return err
}

func (sf *StringFlag) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, sf.parseField)
}

type StringTableStatistics struct {
	StartTime                    int64
	BucketCount                  int64
	EntryCount                   int64
	TotalFootprint               int64
	BucketCountMaximum           int64
	BucketCountAverage           float32
	BucketCountVariance          float32
	BucketCountStandardDeviation float32
	InsertionRate                float32
	RemovalRate                  float32
}

func (sts *StringTableStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		sts.StartTime, err = toLong(r)
	case "bucketCount":
		sts.BucketCount, err = toLong(r)
	case "entryCount":
		sts.EntryCount, err = toLong(r)
	case "totalFootprint":
		sts.TotalFootprint, err = toLong(r)
	case "bucketCountMaximum":
		sts.BucketCountMaximum, err = toLong(r)
	case "bucketCountAverage":
		sts.BucketCountAverage, err = toFloat(r)
	case "bucketCountVariance":
		sts.BucketCountVariance, err = toFloat(r)
	case "bucketCountStandardDeviation":
		sts.BucketCountStandardDeviation, err = toFloat(r)
	case "insertionRate":
		sts.InsertionRate, err = toFloat(r)
	case "removalRate":
		sts.RemovalRate, err = toFloat(r)
	}
	return err
}

func (sts *StringTableStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, sts.parseField)
}

type SymbolTableStatistics struct {
	StartTime                    int64
	BucketCount                  int64
	EntryCount                   int64
	TotalFootprint               int64
	BucketCountMaximum           int64
	BucketCountAverage           float32
	BucketCountVariance          float32
	BucketCountStandardDeviation float32
	InsertionRate                float32
	RemovalRate                  float32
}

func (sts *SymbolTableStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		sts.StartTime, err = toLong(r)
	case "bucketCount":
		sts.BucketCount, err = toLong(r)
	case "entryCount":
		sts.EntryCount, err = toLong(r)
	case "totalFootprint":
		sts.TotalFootprint, err = toLong(r)
	case "bucketCountMaximum":
		sts.BucketCountMaximum, err = toLong(r)
	case "bucketCountAverage":
		sts.BucketCountAverage, err = toFloat(r)
	case "bucketCountVariance":
		sts.BucketCountVariance, err = toFloat(r)
	case "bucketCountStandardDeviation":
		sts.BucketCountStandardDeviation, err = toFloat(r)
	case "insertionRate":
		sts.InsertionRate, err = toFloat(r)
	case "removalRate":
		sts.RemovalRate, err = toFloat(r)
	}
	return err
}

func (sts *SymbolTableStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, sts.parseField)
}

type SystemProcess struct {
	StartTime   int64
	PID         string
	CommandLine string
}

func (sp *SystemProcess) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		sp.StartTime, err = toLong(r)
	case "pid":
		sp.PID, err = toString(r)
	case "commandLine":
		sp.CommandLine, err = toString(r)
	}
	return err
}

func (sp *SystemProcess) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, sp.parseField)
}

type ThreadAllocationStatistics struct {
	StartTime int64
	Allocated int64
	Thread    *Thread
}

func (tas *ThreadAllocationStatistics) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		tas.StartTime, err = toLong(r)
	case "allocated":
		tas.Allocated, err = toLong(r)
	case "thread":
		tas.Thread, err = toThread(p)
	}
	return err
}

func (tas *ThreadAllocationStatistics) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, tas.parseField)
}

type ThreadCPULoad struct {
	StartTime   int64
	EventThread *Thread
	User        float32
	System      float32
}

func (tcl *ThreadCPULoad) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		tcl.StartTime, err = toLong(r)
	case "eventThread":
		tcl.EventThread, err = toThread(p)
	case "user":
		tcl.User, err = toFloat(r)
	case "system":
		tcl.System, err = toFloat(r)
	}
	return err
}

func (tcl *ThreadCPULoad) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, tcl.parseField)
}

type ThreadContextSwitchRate struct {
	StartTime  int64
	SwitchRate float32
}

func (tcsr *ThreadContextSwitchRate) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		tcsr.StartTime, err = toLong(r)
	case "switchRate":
		tcsr.SwitchRate, err = toFloat(r)
	}
	return err
}

func (tcsr *ThreadContextSwitchRate) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, tcsr.parseField)
}

type ThreadDump struct {
	StartTime int64
	Result    string
}

func (td *ThreadDump) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		td.StartTime, err = toLong(r)
	case "result":
		td.Result, err = toString(r)
	}
	return err
}

func (td *ThreadDump) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, td.parseField)
}

type ThreadPark struct {
	StartTime   int64
	Duration    int64
	EventThread *Thread
	StackTrace  *StackTrace
	ParkedClass *Class
	Timeout     int64
	Until       int64
	Address     int64
	ContextId   int64
}

func (tp *ThreadPark) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		tp.StartTime, err = toLong(r)
	case "duration":
		tp.Duration, err = toLong(r)
	case "eventThread":
		tp.EventThread, err = toThread(p)
	case "stackTrace":
		tp.StackTrace, err = toStackTrace(p)
	case "parkedClass":
		tp.ParkedClass, err = toClass(p)
	case "timeout":
		tp.Timeout, err = toLong(r)
	case "until":
		tp.Until, err = toLong(r)
	case "address":
		tp.Address, err = toLong(r)
	case "contextId": // todo this one seems to be unimplemented in the profiler yet
		tp.ContextId, err = toLong(r)
	}
	return err
}

func (tp *ThreadPark) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, tp.parseField)
}

type ThreadStart struct {
	StartTime    int64
	EventThread  *Thread
	StackTrace   *StackTrace
	Thread       *Thread
	ParentThread *Thread
}

func (ts *ThreadStart) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ts.StartTime, err = toLong(r)
	case "eventThread":
		ts.EventThread, err = toThread(p)
	case "stackTrace":
		ts.StackTrace, err = toStackTrace(p)
	case "thread":
		ts.Thread, err = toThread(p)
	case "parentThread":
		ts.ParentThread, err = toThread(p)
	}
	return err
}

func (ts *ThreadStart) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ts.parseField)
}

type UnsignedIntFlag struct {
	StartTime int64
	Name      string
	Value     int32
	Origin    *FlagValueOrigin
}

func (uif *UnsignedIntFlag) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		uif.StartTime, err = toLong(r)
	case "name":
		uif.Name, err = toString(r)
	case "value":
		uif.Value, err = toInt(r)
	case "origin":
		uif.Origin, err = toFlagValueOrigin(p)
	}
	return err
}

func (uif *UnsignedIntFlag) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, uif.parseField)
}

type UnsignedLongFlag struct {
	StartTime int64
	Name      string
	Value     int64
	Origin    *FlagValueOrigin
}

func (ulf *UnsignedLongFlag) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ulf.StartTime, err = toLong(r)
	case "name":
		ulf.Name, err = toString(r)
	case "value":
		ulf.Value, err = toLong(r)
	case "origin":
		ulf.Origin, err = toFlagValueOrigin(p)
	}
	return err
}

func (ulf *UnsignedLongFlag) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ulf.parseField)
}

type VirtualizationInformation struct {
	StartTime int64
	Name      string
}

func (vi *VirtualizationInformation) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		vi.StartTime, err = toLong(r)
	case "name":
		vi.Name, err = toString(r)
	}
	return err
}

func (vi *VirtualizationInformation) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, vi.parseField)
}

type YoungGenerationConfiguration struct {
	StartTime int64
	MinSize   int64
	MaxSize   int64
	NewRatio  int32
}

func (ygc *YoungGenerationConfiguration) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		ygc.StartTime, err = toLong(r)
	case "minSize":
		ygc.MinSize, err = toLong(r)
	case "maxSize":
		ygc.MaxSize, err = toLong(r)
	case "newRatio":
		ygc.NewRatio, err = toInt(r)
	}
	return err
}

func (ygc *YoungGenerationConfiguration) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ygc.parseField)
}

type UnsupportedEvent struct{}

func (ue *UnsupportedEvent) parseField(r reader.Reader, name string, p ParseResolvable) error {
	return nil
}

func (ue *UnsupportedEvent) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, ue.parseField)
}

type LiveObject struct {
	StartTime      int64
	EventThread    *Thread
	StackTrace     *StackTrace
	ObjectClass    *Class
	AllocationSize int64
	AllocationTime int64
}

func (oa *LiveObject) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "startTime":
		oa.StartTime, err = toLong(r)
	case "eventThread":
		oa.EventThread, err = toThread(p)
	case "stackTrace":
		oa.StackTrace, err = toStackTrace(p)
	case "objectClass":
		oa.ObjectClass, err = toClass(p)
	case "allocationSize":
		oa.AllocationSize, err = toLong(r)
	case "allocationTime":
		oa.AllocationTime, err = toLong(r)
	}
	return err
}

func (oa *LiveObject) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, nil, true, oa.parseField)
}
