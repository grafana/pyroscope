package python

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/grafana/pyroscope/ebpf/symtab"
)

type ProcInfo struct {
	Version       Version
	PythonMaps    []*symtab.ProcMap
	LibPythonMaps []*symtab.ProcMap
	Musl          []*symtab.ProcMap
	Glibc         []*symtab.ProcMap
	PyMemSampler  []*symtab.ProcMap
}

var rePython = regexp.MustCompile("/.*/((?:lib)?python)(\\d+)\\.(\\d+)(?:[mu]?(?:\\.so)?)?(?:.1.0)?$")

func GetProcInfoFromPID(pid int) (*ProcInfo, error) {
	mapsFD, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, fmt.Errorf("reading proc maps %d: %w", pid, err)
	}
	defer mapsFD.Close()

	info, err := GetProcInfo(bufio.NewScanner(mapsFD))

	if err != nil {
		return nil, fmt.Errorf("GetPythonProcInfo error %s: %w", fmt.Sprintf("/proc/%d/maps", pid), err)
	}
	return info, nil
}

// GetProcInfo parses /proc/pid/map of a python process.
func GetProcInfo(s *bufio.Scanner) (*ProcInfo, error) {
	res := new(ProcInfo)
	i := 0
	for s.Scan() {
		line := s.Bytes()
		m, err := symtab.ParseProcMapLine(line, false)
		if err != nil {
			return res, err
		}
		if m.Pathname != "" {
			matches := rePython.FindAllStringSubmatch(m.Pathname, -1)
			if matches != nil {
				if res.Version.Major == 0 {
					maj, err := strconv.Atoi(matches[0][2])
					if err != nil {
						return res, fmt.Errorf("failed to parse python version %s", m.Pathname)
					}
					min, err := strconv.Atoi(matches[0][3])
					if err != nil {
						return res, fmt.Errorf("failed to parse python version %s", m.Pathname)
					}
					res.Version.Major = maj
					res.Version.Minor = min
				}
				typ := matches[0][1]
				if typ == "python" {
					res.PythonMaps = append(res.PythonMaps, m)
				} else {
					res.LibPythonMaps = append(res.LibPythonMaps, m)
				}

				i += 1
			}
			if strings.Contains(m.Pathname, "/lib/ld-musl-x86_64.so.1") ||
				strings.Contains(m.Pathname, "/lib/ld-musl-aarch64.so.1") {
				res.Musl = append(res.Musl, m)
			}
			if strings.HasSuffix(m.Pathname, "/libc.so.6") {
				res.Glibc = append(res.Glibc, m)
			}
			if strings.Contains(m.Pathname, "libpymemsampler.so") {
				res.PyMemSampler = append(res.PyMemSampler, m)
			}
		}
	}
	if res.LibPythonMaps == nil && res.PythonMaps == nil {
		return res, fmt.Errorf("no python found")
	}
	return res, nil
}
