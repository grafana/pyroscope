// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pprof writes runtime profiling data in the format expected
// by the pprof visualization tool.
//
// Profiling a Go program
//
// The first step to profiling a Go program is to enable profiling.
// Support for profiling benchmarks built with the standard testing
// package is built into go test. For example, the following command
// runs benchmarks in the current directory and writes the CPU and
// memory profiles to cpu.prof and mem.prof:
//
//     go test -cpuprofile cpu.prof -memprofile mem.prof -bench .
//
// To add equivalent profiling support to a standalone program, add
// code like the following to your main function:
//
//    var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
//    var memprofile = flag.String("memprofile", "", "write memory profile to `file`")
//
//    func main() {
//        flag.Parse()
//        if *cpuprofile != "" {
//            f, err := os.Create(*cpuprofile)
//            if err != nil {
//                log.Fatal("could not create CPU profile: ", err)
//            }
//            defer f.Close() // error handling omitted for example
//            if err := pprof.StartCPUProfile(f); err != nil {
//                log.Fatal("could not start CPU profile: ", err)
//            }
//            defer pprof.StopCPUProfile()
//        }
//
//        // ... rest of the program ...
//
//        if *memprofile != "" {
//            f, err := os.Create(*memprofile)
//            if err != nil {
//                log.Fatal("could not create memory profile: ", err)
//            }
//            defer f.Close() // error handling omitted for example
//            runtime.GC() // get up-to-date statistics
//            if err := pprof.WriteHeapProfile(f); err != nil {
//                log.Fatal("could not write memory profile: ", err)
//            }
//        }
//    }
//
// There is also a standard HTTP interface to profiling data. Adding
// the following line will install handlers under the /debug/pprof/
// URL to download live profiles:
//
//    import _ "net/http/pprof"
//
// See the net/http/pprof package for more details.
//
// Profiles can then be visualized with the pprof tool:
//
//    go tool pprof cpu.prof
//
// There are many commands available from the pprof command line.
// Commonly used commands include "top", which prints a summary of the
// top program hot-spots, and "web", which opens an interactive graph
// of hot-spots and their call graphs. Use "help" for information on
// all pprof commands.
//
// For more information about pprof, see
// https://github.com/google/pprof/blob/master/doc/README.md.
package pprof

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
	"unsafe"
)

// printStackRecord prints the function + source line information
// for a single stack trace.
func printStackRecord(w io.Writer, stk []uintptr, allFrames bool) {
	show := allFrames
	frames := runtime.CallersFrames(stk)
	for {
		frame, more := frames.Next()
		name := frame.Function
		if name == "" {
			show = true
			fmt.Fprintf(w, "#\t%#x\n", frame.PC)
		} else if name != "runtime.goexit" && (show || !strings.HasPrefix(name, "runtime.")) {
			// Hide runtime.goexit and any runtime functions at the beginning.
			// This is useful mainly for allocation traces.
			show = true
			fmt.Fprintf(w, "#\t%#x\t%s+%#x\t%s:%d\n", frame.PC, name, frame.PC-frame.Entry, frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	if !show {
		// We didn't print anything; do it again,
		// and this time include runtime functions.
		printStackRecord(w, stk, true)
		return
	}
	fmt.Fprintf(w, "\n")
}

// writeHeapProto writes the current heap profile in protobuf format to w.
func writeHeapProto(w io.Writer, p []runtime.MemProfileRecord, rate int64, defaultSampleType string) error {
	b := newProfileBuilder(w)
	b.pbValueType(tagProfile_PeriodType, "space", "bytes")
	b.pb.int64Opt(tagProfile_Period, rate)
	b.pbValueType(tagProfile_SampleType, "alloc_objects", "count")
	b.pbValueType(tagProfile_SampleType, "alloc_space", "bytes")
	b.pbValueType(tagProfile_SampleType, "inuse_objects", "count")
	b.pbValueType(tagProfile_SampleType, "inuse_space", "bytes")
	if defaultSampleType != "" {
		b.pb.int64Opt(tagProfile_DefaultSampleType, b.stringIndex(defaultSampleType))
	}

	values := []int64{0, 0, 0, 0}
	var locs []uint64
	for _, r := range p {
		hideRuntime := true
		for tries := 0; tries < 2; tries++ {
			stk := r.Stack()
			// For heap profiles, all stack
			// addresses are return PCs, which is
			// what appendLocsForStack expects.
			if hideRuntime {
				for i, addr := range stk {
					if f := runtime.FuncForPC(addr); f != nil && strings.HasPrefix(f.Name(), "runtime.") {
						continue
					}
					// Found non-runtime. Show any runtime uses above it.
					stk = stk[i:]
					break
				}
			}
			locs = b.appendLocsForStack(locs[:0], stk)
			if len(locs) > 0 {
				break
			}
			hideRuntime = false // try again, and show all frames next time.
		}

		values[0], values[1] = scaleHeapSample(r.AllocObjects, r.AllocBytes, rate)
		values[2], values[3] = scaleHeapSample(r.InUseObjects(), r.InUseBytes(), rate)
		var blockSize int64
		if r.AllocObjects > 0 {
			blockSize = r.AllocBytes / r.AllocObjects
		}
		b.pbSample(values, locs, func() {
			if blockSize != 0 {
				b.pbLabel(tagSample_Label, "bytes", "", blockSize)
			}
		})
	}
	b.build()
	return nil
}

// scaleHeapSample adjusts the data from a heap Sample to
// account for its probability of appearing in the collected
// data. heap profiles are a sampling of the memory allocations
// requests in a program. We estimate the unsampled value by dividing
// each collected sample by its probability of appearing in the
// profile. heap profiles rely on a poisson process to determine
// which samples to collect, based on the desired average collection
// rate R. The probability of a sample of size S to appear in that
// profile is 1-exp(-S/R).
func scaleHeapSample(count, size, rate int64) (int64, int64) {
	if count == 0 || size == 0 {
		return 0, 0
	}

	if rate <= 1 {
		// if rate==1 all samples were collected so no adjustment is needed.
		// if rate<1 treat as unknown and skip scaling.
		return count, size
	}

	avgSize := float64(size) / float64(count)
	scale := 1 / (1 - math.Exp(-avgSize/float64(rate)))

	return int64(float64(count) * scale), int64(float64(size) * scale)
}

// WriteHeapProfile is shorthand for Lookup("heap").WriteTo(w, 0).
// It is preserved for backwards compatibility.
func WriteHeapProfile(w io.Writer) error {
	return writeHeap(w, 0)
}

// writeHeap writes the current runtime heap profile to w.
func writeHeap(w io.Writer, debug int) error {
	return writeHeapInternal(w, debug, "")
}

func writeHeapInternal(w io.Writer, debug int, defaultSampleType string) error {
	var memStats *runtime.MemStats
	if debug != 0 {
		// Read mem stats first, so that our other allocations
		// do not appear in the statistics.
		memStats = new(runtime.MemStats)
		runtime.ReadMemStats(memStats)
	}

	// Find out how many records there are (MemProfile(nil, true)),
	// allocate that many records, and get the data.
	// There's a race—more records might be added between
	// the two calls—so allocate a few extra records for safety
	// and also try again if we're very unlucky.
	// The loop should only execute one iteration in the common case.
	var p []runtime.MemProfileRecord
	n, ok := runtime.MemProfile(nil, true)
	for {
		// Allocate room for a slightly bigger profile,
		// in case a few more entries have been added
		// since the call to MemProfile.
		p = make([]runtime.MemProfileRecord, n+50)
		n, ok = runtime.MemProfile(p, true)
		if ok {
			p = p[0:n]
			break
		}
		// Profile grew; try again.
	}

	if debug == 0 {
		return writeHeapProto(w, p, int64(runtime.MemProfileRate), defaultSampleType)
	}

	sort.Slice(p, func(i, j int) bool { return p[i].InUseBytes() > p[j].InUseBytes() })

	b := bufio.NewWriter(w)
	tw := tabwriter.NewWriter(b, 1, 8, 1, '\t', 0)
	w = tw

	var total runtime.MemProfileRecord
	for i := range p {
		r := &p[i]
		total.AllocBytes += r.AllocBytes
		total.AllocObjects += r.AllocObjects
		total.FreeBytes += r.FreeBytes
		total.FreeObjects += r.FreeObjects
	}

	// Technically the rate is MemProfileRate not 2*MemProfileRate,
	// but early versions of the C++ heap profiler reported 2*MemProfileRate,
	// so that's what pprof has come to expect.
	fmt.Fprintf(w, "heap profile: %d: %d [%d: %d] @ heap/%d\n",
		total.InUseObjects(), total.InUseBytes(),
		total.AllocObjects, total.AllocBytes,
		2*runtime.MemProfileRate)

	for i := range p {
		r := &p[i]
		fmt.Fprintf(w, "%d: %d [%d: %d] @",
			r.InUseObjects(), r.InUseBytes(),
			r.AllocObjects, r.AllocBytes)
		for _, pc := range r.Stack() {
			fmt.Fprintf(w, " %#x", pc)
		}
		fmt.Fprintf(w, "\n")
		printStackRecord(w, r.Stack(), false)
	}

	// Print memstats information too.
	// Pprof will ignore, but useful for people
	s := memStats
	fmt.Fprintf(w, "\n# runtime.MemStats\n")
	fmt.Fprintf(w, "# Alloc = %d\n", s.Alloc)
	fmt.Fprintf(w, "# TotalAlloc = %d\n", s.TotalAlloc)
	fmt.Fprintf(w, "# Sys = %d\n", s.Sys)
	fmt.Fprintf(w, "# Lookups = %d\n", s.Lookups)
	fmt.Fprintf(w, "# Mallocs = %d\n", s.Mallocs)
	fmt.Fprintf(w, "# Frees = %d\n", s.Frees)

	fmt.Fprintf(w, "# HeapAlloc = %d\n", s.HeapAlloc)
	fmt.Fprintf(w, "# HeapSys = %d\n", s.HeapSys)
	fmt.Fprintf(w, "# HeapIdle = %d\n", s.HeapIdle)
	fmt.Fprintf(w, "# HeapInuse = %d\n", s.HeapInuse)
	fmt.Fprintf(w, "# HeapReleased = %d\n", s.HeapReleased)
	fmt.Fprintf(w, "# HeapObjects = %d\n", s.HeapObjects)

	fmt.Fprintf(w, "# Stack = %d / %d\n", s.StackInuse, s.StackSys)
	fmt.Fprintf(w, "# MSpan = %d / %d\n", s.MSpanInuse, s.MSpanSys)
	fmt.Fprintf(w, "# MCache = %d / %d\n", s.MCacheInuse, s.MCacheSys)
	fmt.Fprintf(w, "# BuckHashSys = %d\n", s.BuckHashSys)
	fmt.Fprintf(w, "# GCSys = %d\n", s.GCSys)
	fmt.Fprintf(w, "# OtherSys = %d\n", s.OtherSys)

	fmt.Fprintf(w, "# NextGC = %d\n", s.NextGC)
	fmt.Fprintf(w, "# LastGC = %d\n", s.LastGC)
	fmt.Fprintf(w, "# PauseNs = %d\n", s.PauseNs)
	fmt.Fprintf(w, "# PauseEnd = %d\n", s.PauseEnd)
	fmt.Fprintf(w, "# NumGC = %d\n", s.NumGC)
	fmt.Fprintf(w, "# NumForcedGC = %d\n", s.NumForcedGC)
	fmt.Fprintf(w, "# GCCPUFraction = %v\n", s.GCCPUFraction)
	fmt.Fprintf(w, "# DebugGC = %v\n", s.DebugGC)

	tw.Flush()
	return b.Flush()
}

var cpu struct {
	sync.Mutex
	profiling bool
	done      chan bool
}

// StartCPUProfile enables CPU profiling for the current process.
// While profiling, the profile will be buffered and written to w.
// StartCPUProfile returns an error if profiling is already enabled.
//
// On Unix-like systems, StartCPUProfile does not work by default for
// Go code built with -buildmode=c-archive or -buildmode=c-shared.
// StartCPUProfile relies on the SIGPROF signal, but that signal will
// be delivered to the main program's SIGPROF signal handler (if any)
// not to the one used by Go. To make it work, call os/signal.Notify
// for syscall.SIGPROF, but note that doing so may break any profiling
// being done by the main program.
func StartCPUProfile(w io.Writer, hz uint32) error {
	// The runtime routines allow a variable profiling rate,
	// but in practice operating systems cannot trigger signals
	// at more than about 500 Hz, and our processing of the
	// signal is not cheap (mostly getting the stack trace).
	// 100 Hz is a reasonable choice: it is frequent enough to
	// produce useful data, rare enough not to bog down the
	// system, and a nice round number to make it easy to
	// convert sample counts to seconds. Instead of requiring
	// each client to specify the frequency, we hard code it.
	cpu.Lock()
	defer cpu.Unlock()
	if cpu.done == nil {
		cpu.done = make(chan bool)
	}
	// Double-check.
	if cpu.profiling {
		return fmt.Errorf("cpu profiling already in use")
	}
	cpu.profiling = true
	runtime.SetCPUProfileRate(int(hz))
	go profileWriter(w)
	return nil
}

// readProfile, provided by the runtime, returns the next chunk of
// binary CPU profiling stack trace data, blocking until data is available.
// If profiling is turned off and all the profile data accumulated while it was
// on has been returned, readProfile returns eof=true.
// The caller must save the returned data and tags before calling readProfile again.
//go:linkname readProfile runtime/pprof.readProfile
func readProfile() (data []uint64, tags []unsafe.Pointer, eof bool)

func profileWriter(w io.Writer) {
	b := newProfileBuilder(w)
	var err error
	for {
		time.Sleep(100 * time.Millisecond)
		data, tags, eof := readProfile()
		if e := b.addCPUData(data, tags); e != nil && err == nil {
			err = e
		}
		if eof {
			break
		}
	}
	if err != nil {
		// The runtime should never produce an invalid or truncated profile.
		// It drops records that can't fit into its log buffers.
		panic("runtime/pprof: converting profile: " + err.Error())
	}
	b.build()
	cpu.done <- true
}

// StopCPUProfile stops the current CPU profile, if any.
// StopCPUProfile only returns after all the writes for the
// profile have completed.
func StopCPUProfile() {
	cpu.Lock()
	defer cpu.Unlock()

	if !cpu.profiling {
		return
	}
	cpu.profiling = false
	runtime.SetCPUProfileRate(0)
	<-cpu.done
}
