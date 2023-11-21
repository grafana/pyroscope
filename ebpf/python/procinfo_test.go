package python

import (
	"bufio"
	"bytes"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/grafana/pyroscope/ebpf/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonProcInfo(t *testing.T) {
	var maps string
	maps = `55bb7ceb9000-55bb7ceba000 r--p 00000000 fd:01 57017250                   /home/korniltsev/.asdf/installs/python/3.6.0/bin/python3.6
55bb7ceba000-55bb7cebb000 r-xp 00001000 fd:01 57017250                   /home/korniltsev/.asdf/installs/python/3.6.0/bin/python3.6
55bb7cebb000-55bb7cebc000 r--p 00002000 fd:01 57017250                   /home/korniltsev/.asdf/installs/python/3.6.0/bin/python3.6
55bb7cebc000-55bb7cebd000 r--p 00002000 fd:01 57017250                   /home/korniltsev/.asdf/installs/python/3.6.0/bin/python3.6
55bb7cebd000-55bb7cebe000 rw-p 00003000 fd:01 57017250                   /home/korniltsev/.asdf/installs/python/3.6.0/bin/python3.6
55bb7df87000-55bb7e02a000 rw-p 00000000 00:00 0                          [heap]
7f94a5c00000-7f94a6180000 r--p 00000000 fd:01 45623802                   /usr/lib/locale/locale-archive
7f94a619b000-7f94a6200000 rw-p 00000000 00:00 0 
7f94a6200000-7f94a6222000 r--p 00000000 fd:01 45643280                   /usr/lib/x86_64-linux-gnu/libc.so.6
7f94a6222000-7f94a639a000 r-xp 00022000 fd:01 45643280                   /usr/lib/x86_64-linux-gnu/libc.so.6
7f94a639a000-7f94a63f2000 r--p 0019a000 fd:01 45643280                   /usr/lib/x86_64-linux-gnu/libc.so.6
7f94a63f2000-7f94a63f6000 r--p 001f1000 fd:01 45643280                   /usr/lib/x86_64-linux-gnu/libc.so.6
7f94a63f6000-7f94a63f8000 rw-p 001f5000 fd:01 45643280                   /usr/lib/x86_64-linux-gnu/libc.so.6
7f94a63f8000-7f94a6405000 rw-p 00000000 00:00 0 
7f94a6417000-7f94a6517000 rw-p 00000000 00:00 0 
7f94a6517000-7f94a6525000 r--p 00000000 fd:01 45643925                   /usr/lib/x86_64-linux-gnu/libm.so.6
7f94a6525000-7f94a65a3000 r-xp 0000e000 fd:01 45643925                   /usr/lib/x86_64-linux-gnu/libm.so.6
7f94a65a3000-7f94a65fe000 r--p 0008c000 fd:01 45643925                   /usr/lib/x86_64-linux-gnu/libm.so.6
7f94a65fe000-7f94a65ff000 r--p 000e6000 fd:01 45643925                   /usr/lib/x86_64-linux-gnu/libm.so.6
7f94a65ff000-7f94a6600000 rw-p 000e7000 fd:01 45643925                   /usr/lib/x86_64-linux-gnu/libm.so.6
7f94a6600000-7f94a665c000 r--p 00000000 fd:01 57017251                   /home/korniltsev/.asdf/installs/python/3.6.0/lib/libpython3.6m.so.1.0
7f94a665c000-7f94a67f3000 r-xp 0005c000 fd:01 57017251                   /home/korniltsev/.asdf/installs/python/3.6.0/lib/libpython3.6m.so.1.0
7f94a67f3000-7f94a6886000 r--p 001f3000 fd:01 57017251                   /home/korniltsev/.asdf/installs/python/3.6.0/lib/libpython3.6m.so.1.0
7f94a6886000-7f94a6889000 r--p 00285000 fd:01 57017251                   /home/korniltsev/.asdf/installs/python/3.6.0/lib/libpython3.6m.so.1.0
7f94a6889000-7f94a68ef000 rw-p 00288000 fd:01 57017251                   /home/korniltsev/.asdf/installs/python/3.6.0/lib/libpython3.6m.so.1.0
7f94a68ef000-7f94a6920000 rw-p 00000000 00:00 0 
7f94a693e000-7f94a69c0000 rw-p 00000000 00:00 0 
7f94a69d3000-7f94a69da000 r--s 00000000 fd:01 45645140                   /usr/lib/x86_64-linux-gnu/gconv/gconv-modules.cache
7f94a69da000-7f94a69dc000 rw-p 00000000 00:00 0 
7f94a69dc000-7f94a69dd000 r--p 00000000 fd:01 45642507                   /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f94a69dd000-7f94a6a05000 r-xp 00001000 fd:01 45642507                   /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f94a6a05000-7f94a6a0f000 r--p 00029000 fd:01 45642507                   /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f94a6a0f000-7f94a6a11000 r--p 00033000 fd:01 45642507                   /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f94a6a11000-7f94a6a13000 rw-p 00035000 fd:01 45642507                   /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffc422c4000-7ffc422e6000 rw-p 00000000 00:00 0                          [stack]
7ffc423a6000-7ffc423aa000 r--p 00000000 00:00 0                          [vvar]
7ffc423aa000-7ffc423ac000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]`
	info, err := GetProcInfo(bufio.NewScanner(bytes.NewReader([]byte(maps))))
	require.NoError(t, err)
	require.Nil(t, info.Musl)
	require.Equal(t, info.Version, Version{3, 6, 0})
	require.NotNil(t, info.PythonMaps)
	require.NotNil(t, info.LibPythonMaps)

	maps = `55c07863f000-55c078640000 r--p 00000000 00:32 39877587                   /usr/bin/python3.11
55c078640000-55c078641000 r-xp 00001000 00:32 39877587                   /usr/bin/python3.11
55c078641000-55c078642000 r--p 00002000 00:32 39877587                   /usr/bin/python3.11
55c078642000-55c078643000 r--p 00002000 00:32 39877587                   /usr/bin/python3.11
55c078643000-55c078644000 rw-p 00003000 00:32 39877587                   /usr/bin/python3.11
55c079447000-55c079448000 ---p 00000000 00:00 0                          [heap]
55c079448000-55c07944a000 rw-p 00000000 00:00 0                          [heap]
7f62329de000-7f6232a17000 rw-p 00000000 00:00 0 
7f6232a1e000-7f6232a26000 rw-p 00000000 00:00 0 
7f6232a27000-7f6232b2d000 rw-p 00000000 00:00 0 
7f6232b30000-7f6232b3e000 rw-p 00000000 00:00 0 
7f6232b3f000-7f6232b4a000 rw-p 00000000 00:00 0 
7f6232b4a000-7f6232b4b000 r--p 00000000 00:32 39878051                   /usr/lib/python3.11/lib-dynload/_opcode.cpython-311-x86_64-linux-musl.so
7f6232b4b000-7f6232b4c000 r-xp 00001000 00:32 39878051                   /usr/lib/python3.11/lib-dynload/_opcode.cpython-311-x86_64-linux-musl.so
7f6232b4c000-7f6232b4d000 r--p 00002000 00:32 39878051                   /usr/lib/python3.11/lib-dynload/_opcode.cpython-311-x86_64-linux-musl.so
7f6232b4d000-7f6232b4e000 r--p 00002000 00:32 39878051                   /usr/lib/python3.11/lib-dynload/_opcode.cpython-311-x86_64-linux-musl.so
7f6232b4e000-7f6232b4f000 rw-p 00003000 00:32 39878051                   /usr/lib/python3.11/lib-dynload/_opcode.cpython-311-x86_64-linux-musl.so
7f6232b4f000-7f6232b95000 rw-p 00000000 00:00 0 
7f6232b96000-7f6232c28000 rw-p 00000000 00:00 0 
7f6232c28000-7f6232c35000 r--p 00000000 00:32 39877572                   /usr/lib/libncursesw.so.6.4
7f6232c35000-7f6232c5f000 r-xp 0000d000 00:32 39877572                   /usr/lib/libncursesw.so.6.4
7f6232c5f000-7f6232c76000 r--p 00037000 00:32 39877572                   /usr/lib/libncursesw.so.6.4
7f6232c76000-7f6232c7b000 r--p 0004d000 00:32 39877572                   /usr/lib/libncursesw.so.6.4
7f6232c7b000-7f6232c7c000 rw-p 00052000 00:32 39877572                   /usr/lib/libncursesw.so.6.4
7f6232c7c000-7f6232c90000 r--p 00000000 00:32 39877577                   /usr/lib/libreadline.so.8.2
7f6232c90000-7f6232cb0000 r-xp 00014000 00:32 39877577                   /usr/lib/libreadline.so.8.2
7f6232cb0000-7f6232cb9000 r--p 00034000 00:32 39877577                   /usr/lib/libreadline.so.8.2
7f6232cb9000-7f6232cbc000 r--p 0003d000 00:32 39877577                   /usr/lib/libreadline.so.8.2
7f6232cbc000-7f6232cc2000 rw-p 00040000 00:32 39877577                   /usr/lib/libreadline.so.8.2
7f6232cc2000-7f6232cc3000 rw-p 00000000 00:00 0 
7f6232cc3000-7f6232cc5000 r--p 00000000 00:32 39878086                   /usr/lib/python3.11/lib-dynload/readline.cpython-311-x86_64-linux-musl.so
7f6232cc5000-7f6232cc7000 r-xp 00002000 00:32 39878086                   /usr/lib/python3.11/lib-dynload/readline.cpython-311-x86_64-linux-musl.so
7f6232cc7000-7f6232cc9000 r--p 00004000 00:32 39878086                   /usr/lib/python3.11/lib-dynload/readline.cpython-311-x86_64-linux-musl.so
7f6232cc9000-7f6232cca000 r--p 00006000 00:32 39878086                   /usr/lib/python3.11/lib-dynload/readline.cpython-311-x86_64-linux-musl.so
7f6232cca000-7f6232ccb000 rw-p 00007000 00:32 39878086                   /usr/lib/python3.11/lib-dynload/readline.cpython-311-x86_64-linux-musl.so
7f6232ccb000-7f6232dd4000 rw-p 00000000 00:00 0 
7f6232dd5000-7f6232faa000 rw-p 00000000 00:00 0 
7f6232faa000-7f6233016000 r--p 00000000 00:32 39877591                   /usr/lib/libpython3.11.so.1.0
7f6233016000-7f62331f8000 r-xp 0006c000 00:32 39877591                   /usr/lib/libpython3.11.so.1.0
7f62331f8000-7f62332e2000 r--p 0024e000 00:32 39877591                   /usr/lib/libpython3.11.so.1.0
7f62332e2000-7f6233311000 r--p 00338000 00:32 39877591                   /usr/lib/libpython3.11.so.1.0
7f6233311000-7f6233441000 rw-p 00367000 00:32 39877591                   /usr/lib/libpython3.11.so.1.0
7f6233441000-7f6233483000 rw-p 00000000 00:00 0 
7f6233483000-7f6233497000 r--p 00000000 00:32 39877100                   /lib/ld-musl-x86_64.so.1
7f6233497000-7f62334e3000 r-xp 00014000 00:32 39877100                   /lib/ld-musl-x86_64.so.1
7f62334e3000-7f6233519000 r--p 00060000 00:32 39877100                   /lib/ld-musl-x86_64.so.1
7f6233519000-7f623351a000 r--p 00095000 00:32 39877100                   /lib/ld-musl-x86_64.so.1
7f623351a000-7f623351b000 rw-p 00096000 00:32 39877100                   /lib/ld-musl-x86_64.so.1
7f623351b000-7f623351e000 rw-p 00000000 00:00 0 
7ffeabbad000-7ffeabbce000 rw-p 00000000 00:00 0                          [stack]
7ffeabbe1000-7ffeabbe5000 r--p 00000000 00:00 0                          [vvar]
7ffeabbe5000-7ffeabbe7000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]`
	info, err = GetProcInfo(bufio.NewScanner(bytes.NewReader([]byte(maps))))
	require.NoError(t, err)
	require.NotNil(t, info.Musl)
	require.Equal(t, info.Version, Version{3, 11, 0})
	require.NotNil(t, info.PythonMaps)
	require.NotNil(t, info.LibPythonMaps)

	maps = `00400000-006d8000 r-xp 00000000 08:01 2278062                            /opt/splunk/bin/python3.7m
006d8000-006d9000 r--p 002d7000 08:01 2278062                            /opt/splunk/bin/python3.7m
006d9000-00742000 rw-p 002d8000 08:01 2278062                            /opt/splunk/bin/python3.7m
00742000-00762000 rw-p 00000000 00:00 0 
02067000-02966000 rw-p 00000000 00:00 0                                  [heap]`
	info, err = GetProcInfo(bufio.NewScanner(bytes.NewReader([]byte(maps))))
	require.NoError(t, err)
	require.Nil(t, info.Musl)
	require.Equal(t, info.Version, Version{3, 7, 0})
	require.NotNil(t, info.PythonMaps)
	require.Nil(t, info.LibPythonMaps)
}

const testdataPath = "../testdata/"

func TestMusl(t *testing.T) {
	testutil.InitGitSubmodule(t)
	testcases := []struct {
		path    string
		version Version
	}{
		{testdataPath + "./alpine-arm64/3.8/lib/ld-musl-aarch64.so.1", Version{1, 1, 19}},
		{testdataPath + "./alpine-arm64/3.11/lib/ld-musl-aarch64.so.1", Version{1, 1, 24}},
		{testdataPath + "./alpine-arm64/3.9/lib/ld-musl-aarch64.so.1", Version{1, 1, 20}},
		{testdataPath + "./alpine-arm64/3.12/lib/ld-musl-aarch64.so.1", Version{1, 1, 24}},
		{testdataPath + "./alpine-arm64/3.10/lib/ld-musl-aarch64.so.1", Version{1, 1, 22}},
		{testdataPath + "./alpine-arm64/3.15/lib/ld-musl-aarch64.so.1", Version{1, 2, 2}},
		{testdataPath + "./alpine-arm64/3.13/lib/ld-musl-aarch64.so.1", Version{1, 2, 2}},
		{testdataPath + "./alpine-arm64/3.17/lib/ld-musl-aarch64.so.1", Version{1, 2, 3}},
		{testdataPath + "./alpine-arm64/3.16/lib/ld-musl-aarch64.so.1", Version{1, 2, 3}},
		{testdataPath + "./alpine-arm64/3.18/lib/ld-musl-aarch64.so.1", Version{1, 2, 4}},
		{testdataPath + "./alpine-arm64/3.14/lib/ld-musl-aarch64.so.1", Version{1, 2, 2}},
		{testdataPath + "./alpine-amd64/3.8/lib/ld-musl-x86_64.so.1", Version{1, 1, 19}},
		{testdataPath + "./alpine-amd64/3.11/lib/ld-musl-x86_64.so.1", Version{1, 1, 24}},
		{testdataPath + "./alpine-amd64/3.9/lib/ld-musl-x86_64.so.1", Version{1, 1, 20}},
		{testdataPath + "./alpine-amd64/3.12/lib/ld-musl-x86_64.so.1", Version{1, 1, 24}},
		{testdataPath + "./alpine-amd64/3.10/lib/ld-musl-x86_64.so.1", Version{1, 1, 22}},
		{testdataPath + "./alpine-amd64/3.15/lib/ld-musl-x86_64.so.1", Version{1, 2, 2}},
		{testdataPath + "./alpine-amd64/3.13/lib/ld-musl-x86_64.so.1", Version{1, 2, 2}},
		{testdataPath + "./alpine-amd64/3.17/lib/ld-musl-x86_64.so.1", Version{1, 2, 3}},
		{testdataPath + "./alpine-amd64/3.16/lib/ld-musl-x86_64.so.1", Version{1, 2, 3}},
		{testdataPath + "./alpine-amd64/3.18/lib/ld-musl-x86_64.so.1", Version{1, 2, 4}},
		{testdataPath + "./alpine-amd64/3.14/lib/ld-musl-x86_64.so.1", Version{1, 2, 2}},
	}
	for _, td := range testcases {
		version, err := GetMuslVersionFromFile(td.path)
		require.NoError(t, err)
		assert.Equal(t, td.version, version)
	}
}

func TestPython(t *testing.T) {
	testutil.InitGitSubmodule(t)
	fs := []string{
		testdataPath + "python-x64/3.7.12/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.9.15/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.8.0/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.7.0/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.8.2/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.8.9/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.10.6/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.9.0/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.8.17/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.9.12/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.6.9/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.15/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.11.0/lib/libpython3.11.so.1.0",
		testdataPath + "python-x64/3.6.7/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.4/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.8.15/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.7.9/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.11.3/lib/libpython3.11.so.1.0",
		testdataPath + "python-x64/3.7.3/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.8.4/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.8.5/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.7.7/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.11.2/lib/libpython3.11.so.1.0",
		testdataPath + "python-x64/3.9.2/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.6.15/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.17/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.7.14/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.9.4/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.6.4/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.10.4/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.10.3/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.10.10/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.6.13/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.10.12/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.9.6/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.9.7/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.11.1/lib/libpython3.11.so.1.0",
		testdataPath + "python-x64/3.10.1/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.8.6/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.9.8/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.10.8/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.9.14/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.7.11/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.6.5/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.13/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.7.10/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.9.17/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.9.9/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.6.14/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.8.12/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.10.5/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.10.7/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.8.8/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.8.13/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.6.10/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.9.5/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.8.16/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.6.12/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.9.10/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.10.11/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.7.2/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.7.1/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.10.9/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.10.2/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.9.16/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.9.13/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.6.8/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.8.14/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.8.3/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.6.3/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.9.1/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.6.1/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.6/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.6.0/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.5/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.8.1/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.6.11/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.8/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.6.2/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.7.16/lib/libpython3.7m.so.1.0",
		testdataPath + "python-x64/3.8.7/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.11.4/lib/libpython3.11.so.1.0",
		testdataPath + "python-x64/3.9.11/lib/libpython3.9.so.1.0",
		testdataPath + "python-x64/3.10.0/lib/libpython3.10.so.1.0",
		testdataPath + "python-x64/3.6.6/lib/libpython3.6m.so.1.0",
		testdataPath + "python-x64/3.8.11/lib/libpython3.8.so.1.0",
		testdataPath + "python-x64/3.8.10/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.7.12/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.5.1/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.9.15/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.8.0/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.7.0/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.8.2/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.8.9/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.5.8/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.5.2/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.10.6/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.9.0/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.8.17/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.9.12/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.6.9/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.7.15/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.11.0/lib/libpython3.11.so.1.0",
		testdataPath + "python-arm64/3.6.7/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.9.18/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.7.4/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.8.15/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.7.9/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.11.3/lib/libpython3.11.so.1.0",
		testdataPath + "python-arm64/3.7.3/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.8.4/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.8.5/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.7.7/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.11.2/lib/libpython3.11.so.1.0",
		testdataPath + "python-arm64/3.9.2/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.6.15/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.5.4/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.11.6/lib/libpython3.11.so.1.0",
		testdataPath + "python-arm64/3.7.17/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.7.14/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.9.4/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.6.4/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.10.4/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.5.10/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.10.3/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.10.10/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.6.13/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.5.9/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.10.12/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.9.6/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.5.7/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.9.7/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.11.1/lib/libpython3.11.so.1.0",
		testdataPath + "python-arm64/3.10.1/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.8.6/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.5.3/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.9.8/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.10.8/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.13.0a1/lib/libpython3.13.so.1.0",
		testdataPath + "python-arm64/3.9.14/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.7.11/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.6.5/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.7.13/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.7.10/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.9.17/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.9.9/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.8.18/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.6.14/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.8.12/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.10.5/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.10.7/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.8.8/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.8.13/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.6.10/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.9.5/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.8.16/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.6.12/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.9.10/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.10.11/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.7.2/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.7.1/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.10.9/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.10.2/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.10.13/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.9.16/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.9.13/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.6.8/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.5.0/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.8.14/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.5.5/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.8.3/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.6.3/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.9.1/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.12.0/lib/libpython3.12.so.1.0",
		testdataPath + "python-arm64/3.6.1/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.7.6/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.6.0/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.7.5/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.8.1/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.6.11/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.7.8/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.6.2/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.7.16/lib/libpython3.7m.so.1.0",
		testdataPath + "python-arm64/3.11.5/lib/libpython3.11.so.1.0",
		testdataPath + "python-arm64/3.8.7/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.11.4/lib/libpython3.11.so.1.0",
		testdataPath + "python-arm64/3.9.11/lib/libpython3.9.so.1.0",
		testdataPath + "python-arm64/3.5.6/lib/libpython3.5m.so.1.0",
		testdataPath + "python-arm64/3.10.0/lib/libpython3.10.so.1.0",
		testdataPath + "python-arm64/3.6.6/lib/libpython3.6m.so.1.0",
		testdataPath + "python-arm64/3.8.11/lib/libpython3.8.so.1.0",
		testdataPath + "python-arm64/3.8.10/lib/libpython3.8.so.1.0",
	}
	for _, f := range fs {
		t.Run(f, func(t *testing.T) {
			re := regexp.MustCompile("(\\d+)\\.(\\d+)\\.(\\d+)")
			m := re.FindStringSubmatch(f)
			require.NotNil(t, m)
			major, _ := strconv.Atoi(m[1])
			minor, _ := strconv.Atoi(m[2])
			patch, _ := strconv.Atoi(m[3])

			fd, err := os.Open(f)
			require.NoError(t, err)
			version, err := GetPythonPatchVersion(fd, Version{major, minor, 0})
			require.NoError(t, err)
			require.Equal(t, version.Patch, patch)
		})
	}
}
