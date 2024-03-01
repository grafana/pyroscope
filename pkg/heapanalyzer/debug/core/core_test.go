// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package core

import (
	"fmt"
	"testing"
)

// loadExample loads a simple core file which resulted from running the
// following program on linux/amd64 with go 1.9.0 (the earliest supported runtime):
//
//	package main
//
//	func main() {
//		_ = *(*int)(nil)
//	}
func loadExample(t *testing.T, useExePath bool) *Process {
	t.Helper()
	var p *Process
	var err error
	if useExePath {
		p, err = Core("testdata/core", "", "testdata/tmp/test")
	} else {
		p, err = Core("testdata/core", "testdata", "")
	}
	if err != nil {
		t.Fatalf("can't load test core file: %s", err)
	}
	return p
}

// TestMappings makes sure we can find and load some data.
func TestMappings(t *testing.T) {
	test := func(t *testing.T, useExePath bool) {
		p := loadExample(t, useExePath)
		s, err := p.Symbols()
		if err != nil {
			t.Errorf("can't read symbols: %s\n", err)
		}

		a := s["main.main"]
		m := p.pageTable.findMapping(a)
		if m == nil {
			t.Errorf("text mapping missing")
		}
		if m.Perm() != Read|Exec {
			t.Errorf("bad code section permissions")
		}
		if opcode := p.ReadUint8(a); opcode != 0x31 {
			// 0x31 = xorl instruction.
			// There's no particular reason why this instruction
			// is first. This just tests that reading code works
			// for our specific test binary.
			t.Errorf("opcode=0x%x, want 0x31", opcode)
		}

		a = s["runtime.class_to_size"]
		m = p.pageTable.findMapping(a)
		if m == nil {
			t.Errorf("data mapping missing")
		}
		if m.Perm() != Read|Write {
			t.Errorf("bad data section permissions")
		}
		if size := p.ReadUint16(a.Add(2)); size != 8 {
			t.Errorf("class_to_size[1]=%d, want 8", size)
		}
	}

	for _, useExePath := range []bool{false, true} {
		name := fmt.Sprintf("useExePath=%t", useExePath)
		t.Run(name, func(t *testing.T) {
			test(t, useExePath)
		})
	}
}

// TestConfig checks the configuration accessors.
func TestConfig(t *testing.T) {
	p := loadExample(t, false)
	if arch := p.Arch(); arch != "amd64" {
		t.Errorf("arch=%s, want amd64", arch)
	}
	if size := p.PtrSize(); size != 8 {
		t.Errorf("ptrSize=%d, want 8", size)
	}
	if log := p.LogPtrSize(); log != 3 {
		t.Errorf("logPtrSize=%d, want 3", log)
	}
	if bo := p.ByteOrder(); bo.String() != "LittleEndian" {
		t.Errorf("got %s, want LittleEndian", bo)
	}
}

// TestThread makes sure we get information about running threads.
func TestThread(t *testing.T) {
	p := loadExample(t, true)
	syms, err := p.Symbols()
	if err != nil {
		t.Errorf("can't read symbols: %s\n", err)
	}
	raise := syms["runtime.raise"]
	var size int64 = 1 << 30
	for _, a := range syms {
		if a > raise && a.Sub(raise) < size {
			size = a.Sub(raise)
		}
	}
	found := false
	for _, thr := range p.Threads() {
		if thr.PC() >= raise && thr.PC() < raise.Add(size) {
			found = true
		}
	}
	if !found {
		t.Errorf("can't find thread that did runtime.raise")
	}
}

func TestArgs(t *testing.T) {
	p := loadExample(t, true)
	if got := p.Args(); got != "./test" {
		// this is how the program of testdata/core was invoked.
		t.Errorf("Args() = %q, want './test'", got)
	}
}
