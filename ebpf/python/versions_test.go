package python

import (
	"testing"

	"github.com/grafana/pyroscope/ebpf/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

func TestGlibcVersions(t *testing.T) {
	testutil.InitGitSubmodule(t)
	testdata := []struct {
		path    string
		version Version
	}{
		{"../testdata/glibc-arm64/glibc-2.31/libc.so.6", Version{2, 31, 0}},
		{"../testdata/glibc-arm64/glibc-2.36/libc.so.6", Version{2, 36, 0}},
		{"../testdata/glibc-arm64/glibc-2.29/libc.so.6", Version{2, 29, 0}},
		{"../testdata/glibc-arm64/glibc-2.37/libc.so.6", Version{2, 37, 0}},
		{"../testdata/glibc-arm64/glibc-2.34/libc.so.6", Version{2, 34, 0}},
		{"../testdata/glibc-arm64/glibc-2.32/libc.so.6", Version{2, 32, 0}},
		{"../testdata/glibc-arm64/glibc-2.27/libc.so.6", Version{2, 27, 0}},
		{"../testdata/glibc-arm64/glibc-2.28/libc.so.6", Version{2, 28, 0}},
		{"../testdata/glibc-arm64/glibc-2.35/libc.so.6", Version{2, 35, 0}},
		{"../testdata/glibc-arm64/glibc-2.30/libc.so.6", Version{2, 30, 0}},
		{"../testdata/glibc-arm64/glibc-2.33/libc.so.6", Version{2, 33, 0}},
		{"../testdata/glibc-arm64/glibc-2.38/libc.so.6", Version{2, 38, 0}},
		{"../testdata/glibc-x64/glibc-2.31/libc.so.6", Version{2, 31, 0}},
		{"../testdata/glibc-x64/glibc-2.36/libc.so.6", Version{2, 36, 0}},
		{"../testdata/glibc-x64/glibc-2.29/libc.so.6", Version{2, 29, 0}},
		{"../testdata/glibc-x64/glibc-2.37/libc.so.6", Version{2, 37, 0}},
		{"../testdata/glibc-x64/glibc-2.34/libc.so.6", Version{2, 34, 0}},
		{"../testdata/glibc-x64/glibc-2.32/libc.so.6", Version{2, 32, 0}},
		{"../testdata/glibc-x64/glibc-2.27/libc.so.6", Version{2, 27, 0}},
		{"../testdata/glibc-x64/glibc-2.28/libc.so.6", Version{2, 28, 0}},
		{"../testdata/glibc-x64/glibc-2.35/libc.so.6", Version{2, 35, 0}},
		{"../testdata/glibc-x64/glibc-2.30/libc.so.6", Version{2, 30, 0}},
		{"../testdata/glibc-x64/glibc-2.33/libc.so.6", Version{2, 33, 0}},
		{"../testdata/glibc-x64/glibc-2.38/libc.so.6", Version{2, 38, 0}},
	}
	for _, testdatum := range testdata {
		version, err := GetGlibcVersionFromFile(testdatum.path)
		assert.NoError(t, err)
		assert.Equal(t, testdatum.version, version)

	}
}

func TestGlibcGuess(t *testing.T) {
	orig := glibcOffsets
	defer func() {
		glibcOffsets = orig
	}()
	glibcOffsets = map[Version]*GlibcOffsets{}
	v30 := Version{2, 30, 0}
	glibcOffsets[v30] = orig[v30]
	expected := PerfLibc{
		PthreadSize:             orig[v30].PthreadSize,
		PthreadSpecific1stblock: orig[v30].PthreadSpecific1stblock,
	}
	offsets, guess, err := GetGlibcOffsets(v30)
	assert.NoError(t, err)
	assert.False(t, guess)
	assert.Equal(t, expected, offsets)

	offsets, guess, err = GetGlibcOffsets(Version{2, 31, 0})
	assert.NoError(t, err)
	assert.True(t, guess)
	assert.Equal(t, expected, offsets)
	_, _, err = GetGlibcOffsets(Version{2, 27, 0})
	assert.Error(t, err)
}

func TestKeyData(t *testing.T) {
	for version, offsets := range glibcOffsets {
		assert.Equal(t, int16(8), offsets.PthreadKeyDataData, version.String())
		assert.Equal(t, int16(16), offsets.PthreadKeyDataSize, version.String())
	}
}

type versionOffset struct {
	version Version
	offsets *UserOffsets
}

func sortVersionOffsets(offsets []versionOffset) {
	slices.SortFunc(offsets, func(i, j versionOffset) int {
		return i.version.Compare(&j.version)
	})
}

func sortedVersionOffsets() []versionOffset {
	var offsets []versionOffset
	for version, o := range pyVersions {
		offsets = append(offsets, versionOffset{version, o})
	}
	sortVersionOffsets(offsets)
	return offsets
}

func TestPythonVersionsChangesWithinPatches(t *testing.T) {
	knownChanges := map[Version]struct{}{
		{3, 7, 3}: {},
	}

	m := map[Version][]versionOffset{}
	require.NotEmpty(t, pyVersions)
	for version, offsets := range pyVersions {
		minorVersion := version
		minorVersion.Patch = 0
		m[minorVersion] = append(m[minorVersion], versionOffset{version, offsets})
	}
	for _, offsets := range m {
		sortVersionOffsets(offsets)
		it := offsets[0]
		rest := offsets[1:]
		for len(rest) > 0 {
			next := rest[0]
			if _, ok := knownChanges[it.version]; !ok {
				assert.Equal(t, it.offsets, next.offsets, it.version.String())
			}
			rest = rest[1:]
			it = next
		}
	}
}

func TestPythonVersionFields(t *testing.T) {

	for _, it := range sortedVersionOffsets() {
		version := it.version
		offsets := it.offsets
		t.Run(it.version.String(), func(t *testing.T) {
			present := func(offset int16) {
				t.Helper()
				assert.True(t, offset >= 0)
			}
			missing := func(offset int16) {
				t.Helper()
				assert.False(t, offset >= 0)
			}
			if version.Compare(Py311) >= 0 {
				if version.Compare(Py313) >= 0 {
					present(offsets.PyInterpreterFrame_f_executable)
					missing(offsets.PyInterpreterFrame_f_code)
				} else {
					present(offsets.PyInterpreterFrame_f_code)
					missing(offsets.PyInterpreterFrame_f_executable)
				}
				present(offsets.PyInterpreterFrame_previous)
				present(offsets.PyInterpreterFrame_localsplus)
				//missing(offsets.PyFrameObject_f_back) //todo why??
				missing(offsets.PyFrameObject_f_localsplus)
			} else {
				present(offsets.PyFrameObject_f_code)
				present(offsets.PyFrameObject_f_back)
				present(offsets.PyFrameObject_f_localsplus)
				missing(offsets.PyInterpreterFrame_previous)
				missing(offsets.PyInterpreterFrame_localsplus)
			}

			if version.Compare(Py37) >= 0 {
				assert.Equal(t, int16(0), offsets.PyTssT_is_initialized)
				assert.Equal(t, int16(4), offsets.PyTssT_key)
				assert.Equal(t, int16(8), offsets.PyTssTSize)

				if version.Compare(Py312) >= 0 {
					present(offsets.PyRuntimeState_autoTSSkey)
					present(offsets.PyRuntimeState_gilstate) // we don't actually use it but it is there
					missing(offsets.Gilstate_runtime_state_autoTSSkey)
				} else {
					missing(offsets.PyRuntimeState_autoTSSkey)
					present(offsets.PyRuntimeState_gilstate)
					present(offsets.Gilstate_runtime_state_autoTSSkey)
				}
			} else {
				// using autoTLSkey from bss, not an offset
			}

			//PyVarObject_ob_size               int16
			present(offsets.PyVarObject_ob_size)
			present(offsets.PyObject_ob_type)
			present(offsets.PyTypeObject_tp_name)
			present(offsets.PyTupleObject_ob_item)
			if version.Compare(Py311) >= 0 && version.Compare(Py313) < 0 {
				assert.True(t, offsets.PyThreadState_cframe >= 0)
				assert.True(t, offsets.PyCFrame_current_frame >= 0)
			} else {
				if version.Compare(Py313) >= 0 {
					missing(offsets.PyThreadState_cframe)
					missing(offsets.PyCFrame_current_frame)
					missing(offsets.PyThreadState_frame)
					// PyCFrame was removed in 3.13, lets pretend it was never there and frame field was just renamed to current_frame
					present(offsets.PyThreadState_current_frame)
				} else {
					if version.Compare(Py310) >= 0 {
						present(offsets.PyThreadState_cframe) // we don't use it anyway
					} else {
						missing(offsets.PyThreadState_cframe)
					}
					missing(offsets.PyCFrame_current_frame)
					present(offsets.PyThreadState_frame)
				}
			}

			if version.Compare(Py311) >= 0 {
				present(offsets.PyInterpreterFrame_owner)
			} else {
				missing(offsets.PyInterpreterFrame_owner)
			}
			present(offsets.PyASCIIObjectSize)
			present(offsets.PyCompactUnicodeObjectSize)
			if version.Compare(Py311) >= 0 {
				present(offsets.PyCodeObject_co_localsplusnames)
				missing(offsets.PyCodeObject_co_varnames)
			} else {
				present(offsets.PyCodeObject_co_varnames)
				missing(offsets.PyCodeObject_co_localsplusnames)
			}
			present(offsets.PyCodeObject_co_filename)
			present(offsets.PyCodeObject_co_name)
		})
	}
}
