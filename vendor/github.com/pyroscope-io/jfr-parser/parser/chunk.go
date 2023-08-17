package parser

import (
	"fmt"
	"io"

	"github.com/pyroscope-io/jfr-parser/reader"
)

var magic = []byte{'F', 'L', 'R', 0}

type CPool struct {
	Pool     map[int]ParseResolvable
	resolved bool
}
type ClassMap map[int]*ClassMetadata
type PoolMap map[int]*CPool

type Chunk struct {
	Header      Header
	Metadata    MetadataEvent
	Checkpoints []CheckpointEvent

	// current event
	Event Parseable

	pointer int64
	events  map[int64]int32
	rd      reader.Reader
	err     error
	classes ClassMap
	cpools  map[int]*CPool

	// Cache for common record types
	executionSample           ExecutionSample
	threadPark                ThreadPark
	objectAllocationInNewTLAB ObjectAllocationInNewTLAB
	cpuLoad                   CPULoad
	activeSetting             ActiveSetting
	initialSystemProperty     InitialSystemProperty
	nativeLibrary             NativeLibrary
}

type ChunkParseOptions struct {
	CPoolProcessor     func(meta *ClassMetadata, cpool *CPool)
	UnsafeByteToString bool
}

func (c *Chunk) Parse(r io.Reader, options *ChunkParseOptions) (err error) {
	buf := make([]byte, len(magic))
	if _, err = io.ReadFull(r, buf); err != nil {
		if err == io.EOF {
			return err
		}
		return fmt.Errorf("unable to read chunk's header: %w", err)
	}

	// TODO magic header
	for i, r := range magic {
		if r != buf[i] {
			return fmt.Errorf("unexpected magic header %v expected, %v found", magic, buf)
		}
	}
	if _, err = io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("unable to read format version: %w", err)
	}
	// TODO Check supported major / minor

	buf = make([]byte, headerSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("unable to read chunk header: %w", err)
	}
	if err := c.Header.Parse(reader.NewReader(buf, false, options.UnsafeByteToString)); err != nil {
		return fmt.Errorf("unable to parse chunk header: %w", err)
	}
	c.Header.ChunkSize -= headerSize + 8
	c.Header.MetadataOffset -= headerSize + 8
	c.Header.ConstantPoolOffset -= headerSize + 8
	useCompression := c.Header.Features&1 == 1
	// TODO: assert c.Header.ChunkSize is small enough
	buf = make([]byte, c.Header.ChunkSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("unable to read chunk contents: %w", err)
	}

	rd := reader.NewReader(buf, useCompression, options.UnsafeByteToString)
	pointer := int64(0)
	events := make(map[int64]int32)

	// Parse metadata
	rd.SeekStart(c.Header.MetadataOffset)
	metadataSize, err := rd.VarInt()
	if err != nil {
		return fmt.Errorf("unable to parse chunk metadata size: %w", err)
	}
	events[c.Header.MetadataOffset] = metadataSize
	var metadata MetadataEvent
	if err := metadata.Parse(rd); err != nil {
		return fmt.Errorf("unable to parse chunk metadata: %w", err)
	}
	classes := c.buildClasses(metadata)

	// Parse checkpoint event(s)
	rd.SeekStart(c.Header.ConstantPoolOffset)
	checkpointsSize := int32(0)
	cpools := make(PoolMap)
	delta := int64(0)
	for {
		size, err := rd.VarInt()
		if err != nil {
			return fmt.Errorf("unable to parse checkpoint event size: %w", err)
		}
		events[c.Header.ConstantPoolOffset+delta] = size
		checkpointsSize += size
		var cp CheckpointEvent
		if err := cp.Parse(rd, classes, cpools); err != nil {
			return fmt.Errorf("unable to parse checkpoint event: %w", err)
		}
		c.Checkpoints = append(c.Checkpoints, cp)
		if cp.Delta == 0 {
			break
		}
		delta += cp.Delta
		rd.SeekStart(c.Header.ConstantPoolOffset + delta)
	}

	if options.CPoolProcessor != nil {
		for classID, pool := range cpools {
			options.CPoolProcessor(classes[classID], pool)
		}
	}

	// Second pass over constant pools: resolve constants
	for classID := range cpools {
		if err := ResolveConstants(classes, cpools, classID); err != nil {
			return err
		}
	}

	// Parse the rest of events
	rd.SeekStart(pointer)
	c.rd = rd
	c.classes = classes
	c.cpools = cpools
	c.events = events
	return nil
}

// Next fetches the next event into r.Event.  It returns true if
// successful, and false if it reaches the end of the event stream or
// encounters an error.
//
// The record stored in r.Event may be reused by later invocations of
// Next, so if the caller may need the event after another call to
// Next, it must make its own copy.
func (c *Chunk) Next() bool {
	if c.err != nil {
		return false
	}
	for c.pointer != c.Header.ChunkSize {
		if size, ok := c.events[c.pointer]; ok {
			c.pointer += int64(size)
		} else {
			if _, err := c.rd.SeekStart(c.pointer); err != nil {
				c.err = fmt.Errorf("unable to seek to position %d: %w", c.pointer, err)
				return false
			}
			size, err := c.rd.VarInt()
			if err != nil {
				c.err = fmt.Errorf("unable to parse event size: %w", err)
				return false
			}
			e, err := ParseEvent(c.rd, c.classes, c.cpools)
			if err != nil {
				c.err = fmt.Errorf("unable to parse event: %w", err)
				return false
			}
			c.Event = e
			c.pointer += int64(size)
			return true
		}
	}
	return false
}

// Err returns the first error encountered by Events.
func (c *Chunk) Err() error {
	return c.err
}

func (c *Chunk) buildClasses(metadata MetadataEvent) ClassMap {
	cacheEventFns := map[string]func() Parseable{
		"jdk.CPULoad": func() Parseable {
			c.cpuLoad = CPULoad{}
			return &c.cpuLoad
		},
		"jdk.ThreadPark": func() Parseable {
			c.threadPark = ThreadPark{}
			return &c.threadPark
		},
		"jdk.ExecutionSample": func() Parseable {
			c.executionSample = ExecutionSample{} // threadstate
			return &c.executionSample
		},
		"jdk.ObjectAllocationInNewTLAB": func() Parseable {
			c.objectAllocationInNewTLAB = ObjectAllocationInNewTLAB{}
			return &c.objectAllocationInNewTLAB
		},
		"jdk.InitialSystemProperty": func() Parseable {
			c.initialSystemProperty = InitialSystemProperty{}
			return &c.initialSystemProperty
		},
		"jdk.ActiveSetting": func() Parseable {
			c.activeSetting = ActiveSetting{}
			return &c.activeSetting
		},
		"jdk.NativeLibrary": func() Parseable {
			c.nativeLibrary = NativeLibrary{}
			return &c.nativeLibrary
		},
	}
	classes := make(map[int]*ClassMetadata, len(metadata.Root.Metadata.Classes))
	for i := range metadata.Root.Metadata.Classes {
		class := &metadata.Root.Metadata.Classes[i]
		var numConstants int
		for _, field := range class.Fields {
			if field.ConstantPool {
				numConstants++
			}
		}

		if cacheEventFn, ok := cacheEventFns[class.Name]; ok {
			class.eventFn = cacheEventFn
		} else if eventFn, ok := events[class.Name]; ok {
			class.eventFn = eventFn
		} else {
			class.eventFn = func() Parseable {
				return &UnsupportedEvent{}
			}
		}

		if typeFn, ok := types[class.Name]; ok {
			class.typeFn = typeFn
		} else {
			class.typeFn = func() ParseResolvable {
				return &UnsupportedType{}
			}
		}
		class.numConstants = numConstants
		classes[int(class.ID)] = class
	}

	// init class field isBaseType
	for i, class := range metadata.Root.Metadata.Classes {
		for j, field := range class.Fields {
			name := classes[int(field.Class)].Name
			if _, ok := parseBaseTypeAndDrops[name]; ok {
				metadata.Root.Metadata.Classes[i].Fields[j].isBaseType = true
				metadata.Root.Metadata.Classes[i].Fields[j].parseBaseTypeAndDrop = parseBaseTypeAndDrops[name]
			}
		}
	}
	return classes
}

func ResolveConstants(classes ClassMap, cpools PoolMap, classID int) (err error) {
	cpool, ok := cpools[classID]
	if !ok {
		// Non-existent constant pool references seem to be used to mark no value
		return nil
	}
	if cpool.resolved {
		return nil
	}
	cpool.resolved = true
	for _, t := range cpool.Pool {
		if err := t.Resolve(classes, cpools); err != nil {
			return fmt.Errorf("unable to resolve constants: %w", err)
		}
	}
	return nil
}
