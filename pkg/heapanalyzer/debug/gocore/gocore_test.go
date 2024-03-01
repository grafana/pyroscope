// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package gocore

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/testenv"
)

// loadTest loads a simple core file which resulted from running the
// following program on linux/amd64 with go 1.9.0 (the earliest supported runtime):
//
//	package main
//
//	func main() {
//		_ = *(*int)(nil)
//	}
func loadExample(t *testing.T) *Process {
	t.Helper()
	if runtime.GOOS == "android" {
		t.Skip("skipping test on android")
	}
	c, err := core.Core("testdata/core", "testdata", "")
	if err != nil {
		t.Fatalf("can't load test core file: %s", err)
	}
	p, err := Core(c)
	if err != nil {
		t.Fatalf("can't parse Go core: %s", err)
	}
	return p
}

func loadExampleVersion(t *testing.T, version string) *Process {
	t.Helper()
	if runtime.GOOS == "android" {
		t.Skip("skipping test on android")
	}
	if version == "1.9" {
		version = ""
	}
	var file string
	var base string
	if strings.HasSuffix(version, ".zip") {
		// Make temporary directory.
		dir, err := os.MkdirTemp("", strings.TrimSuffix(version, ".zip")+"_")
		if err != nil {
			t.Fatalf("can't make temp directory: %s", err)
		}
		defer os.RemoveAll(dir)

		// Unpack test into directory.
		unzip(t, filepath.Join("testdata", version), dir)

		file = filepath.Join(dir, "tmp", "coretest", "core")
		base = dir
	} else {
		file = fmt.Sprintf("testdata/core%s", version)
		base = "testdata"
	}
	c, err := core.Core(file, base, "")
	if err != nil {
		t.Fatalf("can't load test core file: %s", err)
	}
	p, err := Core(c)
	if err != nil {
		t.Fatalf("can't parse Go core: %s", err)
	}
	p.TypeHeap()
	return p
}

// loadExampleGenerated generates a core from a binary built with
// runtime.GOROOT().
func loadExampleGenerated(t *testing.T) *Process {
	t.Helper()
	testenv.MustHaveGoBuild(t)
	switch runtime.GOOS {
	case "js", "plan9", "windows":
		t.Skipf("skipping: no core files on %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" {
		t.Skipf("skipping: only parsing of amd64 cores is supported")
	}

	cleanup := setupCorePattern(t)
	defer cleanup()

	dir := t.TempDir()
	file, output, err := generateCore(dir)
	t.Logf("crasher output: %s", output)
	if err != nil {
		t.Fatalf("generateCore() got err %v want nil", err)
	}
	c, err := core.Core(file, "", "")
	if err != nil {
		t.Fatalf("can't load test core file: %s", err)
	}
	p, err := Core(c)
	if err != nil {
		t.Fatalf("can't parse Go core: %s", err)
	}
	return p
}

func setupCorePattern(t *testing.T) func() {
	if runtime.GOOS != "linux" {
		t.Skip("skipping: core file pattern check implemented only for Linux")
	}

	const (
		corePatternPath = "/proc/sys/kernel/core_pattern"
		newPattern      = "core"
	)

	b, err := os.ReadFile(corePatternPath)
	if err != nil {
		t.Fatalf("unable to read core pattern: %v", err)
	}
	pattern := string(b)
	t.Logf("original core pattern: %s", pattern)

	// We want a core file in the working directory containing "core" in
	// the name. If the pattern already matches this, there is nothing to
	// do. What we don't want:
	//  - Pipe to another process
	//  - Path components
	if !strings.HasPrefix(pattern, "|") && !strings.Contains(pattern, "/") && strings.Contains(pattern, "core") {
		// Pattern is fine as-is, nothing to do.
		return func() {}
	}

	if os.Getenv("GO_BUILDER_NAME") == "" {
		// Don't change the core pattern on arbitrary machines, as it
		// has global effect.
		t.Skipf("skipping: unable to generate core file due to incompatible core pattern %q; set %s to %q", pattern, corePatternPath, newPattern)
	}

	t.Logf("updating core pattern to %q", newPattern)

	err = os.WriteFile(corePatternPath, []byte(newPattern), 0)
	if err != nil {
		t.Skipf("skipping: unable to write core pattern: %v", err)
	}

	return func() {
		t.Logf("resetting core pattern to %q", pattern)
		err := os.WriteFile(corePatternPath, []byte(pattern), 0)
		if err != nil {
			t.Errorf("unable to write core pattern back to original value: %v", err)
		}
	}
}

func generateCore(dir string) (string, []byte, error) {
	goTool, err := testenv.GoTool()
	if err != nil {
		return "", nil, fmt.Errorf("cannot find go tool: %w", err)
	}

	const source = "./testdata/coretest/test.go"
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("erroring getting cwd: %w", err)
	}

	srcPath := filepath.Join(cwd, source)
	cmd := exec.Command(goTool, "build", "-o", "test.exe", srcPath)
	cmd.Dir = dir

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("error building crasher: %w\n%s", err, string(b))
	}

	cmd = exec.Command("./test.exe")
	cmd.Env = append(os.Environ(), "GOTRACEBACK=crash")
	cmd.Dir = dir

	b, err = cmd.CombinedOutput()
	// We expect a crash.
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return "", b, fmt.Errorf("crasher did not crash, got err %T %w", err, err)
	}

	// Look for any file with "core" in the name.
	dd, err := os.ReadDir(dir)
	if err != nil {
		return "", b, fmt.Errorf("error reading output directory: %w", err)
	}

	for _, d := range dd {
		if strings.Contains(d.Name(), "core") {
			return filepath.Join(dir, d.Name()), b, nil
		}
	}

	names := make([]string, 0, len(dd))
	for _, d := range dd {
		names = append(names, d.Name())
	}
	return "", b, fmt.Errorf("did not find core file in %+v", names)
}

// unzip unpacks the zip file name into the directory dir.
func unzip(t *testing.T, name, dir string) {
	t.Helper()
	r, err := zip.OpenReader(name)
	if err != nil {
		t.Fatalf("can't read zip file %s: %s", name, err)
	}
	for _, f := range r.File {
		rf, err := f.Open()
		if err != nil {
			t.Fatalf("can't read entry %s: %s", f.Name, err)
		}
		err = os.MkdirAll(path.Dir(filepath.Join(dir, f.Name)), 0777)
		if err != nil {
			t.Fatalf("can't make directory: %s", err)
		}
		wf, err := os.Create(filepath.Join(dir, f.Name))
		if err != nil {
			t.Fatalf("can't write entry %s: %s", f.Name, err)
		}
		_, err = io.Copy(wf, rf)
		if err != nil {
			t.Fatalf("can't copy %s: %s", f.Name, err)
		}
		err = rf.Close()
		if err != nil {
			t.Fatalf("can't close reader %s: %s", f.Name, err)
		}
		err = wf.Close()
		if err != nil {
			t.Fatalf("can't close writer %s: %s", f.Name, err)
		}
	}
}

func TestObjects(t *testing.T) {
	p := loadExample(t)
	n := 0
	p.ForEachObject(func(x Object) bool {
		n++
		return true
	})
	if n != 104 {
		t.Errorf("#objects = %d, want 104", n)
	}
}

func TestRoots(t *testing.T) {
	p := loadExample(t)
	n := 0
	p.ForEachRoot(func(r *Root) bool {
		n++
		return true
	})
	if n != 257 {
		t.Errorf("#roots = %d, want 257", n)
	}
}

// TestConfig checks the configuration accessors.
func TestConfig(t *testing.T) {
	p := loadExample(t)
	if v := p.BuildVersion(); v != "go1.9" {
		t.Errorf("version=%s, wanted go1.9", v)
	}
	if n := p.Stats().Size; n != 2732032 {
		t.Errorf("all stats=%d, want 2732032", n)
	}
}

func TestFindFunc(t *testing.T) {
	p := loadExample(t)
	a := core.Address(0x404000)
	f := p.FindFunc(a)
	if f == nil {
		t.Errorf("can't find function at %x", a)
		return
	}
	if n := f.Name(); n != "runtime.recvDirect" {
		t.Errorf("funcname(%x)=%s, want runtime.recvDirect", a, n)
	}
}

func TestTypes(t *testing.T) {
	p := loadExample(t)
	// Check the type of a few objects.
	for _, s := range [...]struct {
		addr   core.Address
		size   int64
		kind   Kind
		name   string
		repeat int64
	}{
		{0xc420000480, 384, KindStruct, "runtime.g", 1},
		{0xc42000a020, 32, KindPtr, "*runtime.g", 4},
		{0xc420082000, 96, KindStruct, "hchan<bool>", 1},
		{0xc420062000, 64, KindStruct, "runtime._defer", 1},
	} {
		x, i := p.FindObject(s.addr)
		if x == 0 {
			t.Errorf("can't find object at %x", s.addr)
			continue
		}
		if i != 0 {
			t.Errorf("offset(%x)=%d, want 0", s.addr, i)
		}
		if p.Size(x) != s.size {
			t.Errorf("size(%x)=%d, want %d", s.addr, p.Size(x), s.size)
		}
		typ, repeat := p.Type(x)
		if typ.Kind != s.kind {
			t.Errorf("kind(%x)=%s, want %s", s.addr, typ.Kind, s.kind)
		}
		if typ.Name != s.name {
			t.Errorf("name(%x)=%s, want %s", s.addr, typ.Name, s.name)
		}
		if repeat != s.repeat {
			t.Errorf("repeat(%x)=%d, want %d", s.addr, repeat, s.repeat)
		}

		y, i := p.FindObject(s.addr + 1)
		if y != x {
			t.Errorf("can't find object at %x", s.addr+1)
		}
		if i != 1 {
			t.Errorf("offset(%x)=%d, want i", s.addr, i)
		}
	}
}

func TestReverse(t *testing.T) {
	p := loadExample(t)

	// Build the pointer map.
	// m[x]=y means address x has a pointer to address y.
	m1 := map[core.Address]core.Address{}
	p.ForEachObject(func(x Object) bool {
		p.ForEachPtr(x, func(i int64, y Object, j int64) bool {
			m1[p.Addr(x).Add(i)] = p.Addr(y).Add(j)
			return true
		})
		return true
	})
	p.ForEachRoot(func(r *Root) bool {
		p.ForEachRootPtr(r, func(i int64, y Object, j int64) bool {
			m1[r.Addr.Add(i)] = p.Addr(y).Add(j)
			return true
		})
		return true
	})

	// Build the same, with reverse entries.
	m2 := map[core.Address]core.Address{}
	p.ForEachObject(func(y Object) bool {
		p.ForEachReversePtr(y, func(x Object, r *Root, i, j int64) bool {
			if r != nil {
				m2[r.Addr.Add(i)] = p.Addr(y).Add(j)
			} else {
				m2[p.Addr(x).Add(i)] = p.Addr(y).Add(j)
			}
			return true
		})
		return true
	})

	if !reflect.DeepEqual(m1, m2) {
		t.Errorf("forward and reverse edges don't match")
	}
}

func TestDynamicType(t *testing.T) {
	p := loadExample(t)
	for _, g := range p.Globals() {
		if g.Name == "runtime.indexError" {
			d := p.DynamicType(g.Type, g.Addr)
			if d.Name != "runtime.errorString" {
				t.Errorf("dynamic type wrong: got %s want runtime.errorString", d.Name)
			}
		}
	}
}

func TestVersions(t *testing.T) {
	versions := []string{
		"1.10",
		"1.11",
		"1.12.zip",
		"1.13.zip",
		"1.13.3.zip",
		"1.14.zip",
		"1.16.zip",
		"1.17.zip",
		"1.18.zip",
		"1.19.zip",
	}
	for _, ver := range versions {
		t.Run(ver, func(t *testing.T) {
			loadExampleVersion(t, ver)
		})
	}

	t.Run("goroot", func(t *testing.T) {
		t.Skip("doesn't work with Go 1.22 allocation headers yet")
		loadExampleGenerated(t)
	})
}

func loadZipCore(t *testing.T, name string) *Process {
	t.Helper()
	if runtime.GOOS == "android" {
		t.Skip("skipping test on android")
	}
	// Make temporary directory.
	dir, err := os.MkdirTemp("", name+"_")
	if err != nil {
		t.Fatalf("can't make temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// Unpack bin file and core file into directory.
	unzip(t, filepath.Join("testdata", name+".zip"), dir)

	exe := filepath.Join(dir, name)
	file := filepath.Join(dir, "core")
	c, err := core.Core(file, dir, exe)
	if err != nil {
		t.Fatalf("can't load test core file: %s", err)
	}
	p, err := Core(c)
	if err != nil {
		t.Fatalf("can't parse Go core: %s", err)
	}
	return p
}

func TestRuntimeTypes(t *testing.T) {
	p := loadZipCore(t, "runtimetype")
	// Check the type of a few objects.
	for _, s := range [...]struct {
		addr   core.Address
		size   int64
		kind   Kind
		name   string
		repeat int64
	}{
		{0xc00018e000, 16, KindStruct, "example.com/m/path-a/pkg.T1", 1},
		{0xc00018e010, 16, KindStruct, "example.com/m/path-a/pkg.T2", 1},
		{0xc000190000, 32, KindStruct, "example.com/m/path-b/pkg.T1", 1},
		{0xc000190020, 32, KindStruct, "example.com/m/path-b/pkg.T2", 1},
	} {
		x, i := p.FindObject(s.addr)
		if x == 0 {
			t.Errorf("can't find object at %x", s.addr)
			continue
		}
		if i != 0 {
			t.Errorf("offset(%x)=%d, want 0", s.addr, i)
		}
		if p.Size(x) != s.size {
			t.Errorf("size(%x)=%d, want %d", s.addr, p.Size(x), s.size)
		}
		typ, repeat := p.Type(x)
		if typ.Kind != s.kind {
			t.Errorf("kind(%x)=%s, want %s", s.addr, typ.Kind, s.kind)
		}
		if typ.Name != s.name {
			t.Errorf("name(%x)=%s, want %s", s.addr, typ.Name, s.name)
		}
		if repeat != s.repeat {
			t.Errorf("repeat(%x)=%d, want %d", s.addr, repeat, s.repeat)
		}

		y, i := p.FindObject(s.addr + 1)
		if y != x {
			t.Errorf("can't find object at %x", s.addr+1)
		}
		if i != 1 {
			t.Errorf("offset(%x)=%d, want i", s.addr, i)
		}
	}
}
