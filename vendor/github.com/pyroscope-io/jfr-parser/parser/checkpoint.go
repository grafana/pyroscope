package parser

import (
	"fmt"

	"github.com/pyroscope-io/jfr-parser/reader"
)

type CheckpointEvent struct {
	StartTime int64
	Duration  int64
	Delta     int64
	TypeMask  int8
}

func (c *CheckpointEvent) Parse(r reader.Reader, classes ClassMap, cpools PoolMap) (err error) {
	if kind, err := r.VarLong(); err != nil {
		return fmt.Errorf("unable to retrieve event type: %w", err)
	} else if kind != 1 {
		return fmt.Errorf("unexpected checkpoint event type: %d", kind)
	}
	if c.StartTime, err = r.VarLong(); err != nil {
		return fmt.Errorf("unable to parse checkpoint event's start time: %w", err)
	}
	if c.Duration, err = r.VarLong(); err != nil {
		return fmt.Errorf("unable to parse checkpoint event's duration: %w", err)
	}
	if c.Delta, err = r.VarLong(); err != nil {
		return fmt.Errorf("unable to parse checkpoint event's delta: %w", err)
	}
	c.TypeMask, _ = r.Byte()
	n, err := r.VarInt()
	if err != nil {
		return fmt.Errorf("unable to parse checkpoint event's number of constant pools: %w", err)
	}
	// TODO: assert n is small enough
	for i := 0; i < int(n); i++ {
		classID, err := r.VarLong()
		if err != nil {
			return fmt.Errorf("unable to parse constant pool class: %w", err)
		}
		m, err := r.VarInt()
		if err != nil {
			return fmt.Errorf("unable to parse constant pool's number of constants: %w", err)
		}
		cm, ok := cpools[int(classID)]
		if !ok {
			cpools[int(classID)] = &CPool{Pool: make(map[int]ParseResolvable, m)}
			cm = cpools[int(classID)]
		}
		class, ok := classes[int(classID)]
		if !ok {
			return fmt.Errorf("unexpected class %d", classID)
		}
		var (
			results            []ParseResolvable
			preAllocateResults = true
			contantsSlice      = getConstantsSlice(int(m), class.numConstants)
		)
		// preallocate common class in async-profiler
		results = make([]ParseResolvable, m)
		switch class.Name {
		case "java.lang.Thread":
			threads := make([]Thread, m)
			for i := range threads {
				threads[i].constants = contantsSlice[i]
				results[i] = &threads[i]
			}
		case "jdk.types.StackTrace":
			var classStackFrames *ClassMetadata
			for _, class := range classes {
				if class.Name == "jdk.types.StackFrame" {
					classStackFrames = class
					break
				}
			}
			var (
				stackFrames      []StackFrame
				indexStackFrames int
			)
			createStackFrames := func() ParseResolvable {
				if indexStackFrames >= len(stackFrames) {
					stackFrames = make([]StackFrame, m)
					contantsSlice = getConstantsSlice(int(m), classStackFrames.numConstants)
					for i := range stackFrames {
						stackFrames[i].constants = contantsSlice[i]
					}
					indexStackFrames = 0
				}
				result := &stackFrames[indexStackFrames]
				indexStackFrames++
				return result
			}
			for i, class := range classes {
				if class.Name == "jdk.types.StackFrame" {
					class.typeFn = createStackFrames
					classes[i] = class
					break
				}
			}
			var pointerToStackFrames []*StackFrame
			indexPointerToStackFrames := 0
			getPointerToStackFrames := func(n int) []*StackFrame {
				if n > int(m) {
					return make([]*StackFrame, n)[:0]
				}
				if indexPointerToStackFrames+n > len(pointerToStackFrames) {
					pointerToStackFrames = make([]*StackFrame, m)
					indexPointerToStackFrames = 0
				}
				result := pointerToStackFrames[indexPointerToStackFrames : indexPointerToStackFrames+n]
				indexPointerToStackFrames += n
				return result[:0]
			}
			stackTraces := make([]StackTrace, m)
			for i := range stackTraces {
				stackTraces[i].getPointerToStackFrames = getPointerToStackFrames
				stackTraces[i].constants = contantsSlice[i]
				results[i] = &stackTraces[i]
			}
		case "jdk.types.Method":
			methods := make([]Method, m)
			for i := range methods {
				methods[i].constants = contantsSlice[i]
				results[i] = &methods[i]
			}
		case "java.lang.Class":
			classes := make([]Class, m)
			for i := range classes {
				classes[i].constants = contantsSlice[i]
				results[i] = &classes[i]
			}
		case "jdk.types.Package":
			packages := make([]Package, m)
			for i := range packages {
				packages[i].constants = contantsSlice[i]
				results[i] = &packages[i]
			}
		case "jdk.types.Symbol":
			symbols := make([]Symbol, m)
			for i := range symbols {
				symbols[i].constants = contantsSlice[i]
				results[i] = &symbols[i]
			}
		default:
			preAllocateResults = false
		}
		// TODO: assert m is small enough
		for j := 0; j < int(m); j++ {
			idx, err := r.VarLong()
			if err != nil {
				return fmt.Errorf("unable to parse contant's index: %w", err)
			}

			var v ParseResolvable
			if preAllocateResults {
				v = results[j]
				err = v.Parse(r, classes, cpools, class)
			} else {
				v, err = ParseClass(r, classes, cpools, classID)
				if err != nil {
					return fmt.Errorf("unable to parse constant type %d: %w", classID, err)
				}
			}
			cm.Pool[int(idx)] = v
		}
	}
	return nil
}

func getConstantsSlice(size int, num int) [][]constant {
	constants := make([]constant, size*num)
	contantsSlice := make([][]constant, size)
	for i := range contantsSlice {
		contantsSlice[i] = constants[i*num : (i+1)*num][:0]
	}
	return contantsSlice
}
