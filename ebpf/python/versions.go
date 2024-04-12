package python

import (
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
)

type Version struct {
	Major, Minor, Patch int
}

var Py313 = &Version{Major: 3, Minor: 13}
var Py312 = &Version{Major: 3, Minor: 12}
var Py311 = &Version{Major: 3, Minor: 11}
var Py310 = &Version{Major: 3, Minor: 10}
var Py37 = &Version{Major: 3, Minor: 7}

func (p *Version) Compare(other *Version) int {
	major := p.Major - other.Major
	if major != 0 {
		return major
	}

	minor := p.Minor - other.Minor
	if minor != 0 {
		return minor
	}
	return p.Patch - other.Patch
}

func (p *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", p.Major, p.Minor, p.Patch)
}

// GetPythonPatchVersion searches for a patch version given a major + minor version with regexp
// r is libpython3.11.so or python3.11 elf binary
func GetPythonPatchVersion(r io.Reader, v Version) (Version, error) {
	rePythonVersion := regexp.MustCompile(fmt.Sprintf("%d\\.%d\\.(\\d+)[^\\d]", v.Major, v.Minor))
	res := v
	res.Patch = 0
	m, err := rgrep(r, rePythonVersion)
	if err != nil {
		return res, fmt.Errorf("rgrep python version %v %w", v, err)
	}
	patch, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return res, fmt.Errorf("error trying to grep python patch version %s, %w", string(m[0]), err)
	}
	res.Patch = patch
	return res, nil
}

func rgrep(r io.Reader, re *regexp.Regexp) ([][]byte, error) {
	const bufSize = 0x1000
	const lookBack = 0x10
	buf := make([]byte, bufSize+lookBack)
	for {
		n, err := r.Read(buf[lookBack:])
		if n > 0 {
			it := buf[:lookBack+n]
			submatch := re.FindSubmatch(it)
			if submatch != nil {
				return submatch, nil
			}
			copy(buf[:lookBack], it[len(it)-lookBack:])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("error trying to grep python patch version %w", err)
		}
	}
	return nil, fmt.Errorf("rgrep not found %v", re.String())
}

// UserOffsets keeps Python offsets which are then partially passed to ebpf with ProfilePyOffsetConfig
//
//goland:noinspection GoSnakeCaseUsage
type UserOffsets struct {
	PyVarObject_ob_size               int16
	PyObject_ob_type                  int16
	PyTypeObject_tp_name              int16
	PyThreadState_frame               int16
	PyThreadState_cframe              int16
	PyThreadState_current_frame       int16
	PyCFrame_current_frame            int16
	PyFrameObject_f_back              int16
	PyFrameObject_f_code              int16
	PyFrameObject_f_localsplus        int16
	PyCodeObject_co_filename          int16
	PyCodeObject_co_name              int16
	PyCodeObject_co_varnames          int16
	PyCodeObject_co_localsplusnames   int16
	PyTupleObject_ob_item             int16
	PyInterpreterFrame_f_code         int16
	PyInterpreterFrame_f_executable   int16
	PyInterpreterFrame_previous       int16
	PyInterpreterFrame_localsplus     int16
	PyInterpreterFrame_owner          int16
	PyRuntimeState_gilstate           int16
	PyRuntimeState_autoTSSkey         int16
	Gilstate_runtime_state_autoTSSkey int16
	PyTssT_is_initialized             int16
	PyTssT_key                        int16
	PyTssTSize                        int16
	PyASCIIObjectSize                 int16
	PyCompactUnicodeObjectSize        int16
}

type GlibcOffsets struct {
	PthreadSpecific1stblock int16
	PthreadSize             int16
	PthreadKeyDataData      int16
	PthreadKeyDataSize      int16
}

type MuslOffsets struct {
	PthreadTsd  int16
	PthreadSize int16
}

func GetUserOffsets(version Version) (*UserOffsets, bool, error) {
	return getVersionGuessing(version, pyVersions)
}

// getVersionGuessing returns offsets for a given version. If version is not found, it tries to guess the closest one
// within the same major.minor version. If that fails, it returns an error.
func getVersionGuessing[T any](version Version, m map[Version]*T) (*T, bool, error) {
	offsets, ok := m[version]
	patchGuess := false
	if !ok {
		foundVersion := Version{}
		for v, o := range m {
			if v.Major == version.Major && v.Minor == version.Minor {
				if offsets == nil {
					offsets = o
					foundVersion = v
				} else if v.Compare(&foundVersion) > 0 {
					offsets = o
					foundVersion = v
				}
			}
		}
		if offsets == nil {
			return nil, false, fmt.Errorf("unsupported version %v ", version)
		}
		patchGuess = true
	}

	return offsets, patchGuess, nil
}
