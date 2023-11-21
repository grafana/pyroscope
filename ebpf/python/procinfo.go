package python

import (
	"bufio"
	"fmt"
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
}

var rePython = regexp.MustCompile("/.*/((?:lib)?python)(\\d+)\\.(\\d+)(?:[mu]?(?:\\.so)?)?(?:.1.0)?$")

// GetProcInfo parses /proc/pid/map of a python process.
func GetProcInfo(s *bufio.Scanner) (ProcInfo, error) {
	res := ProcInfo{}
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
		}
	}
	if res.LibPythonMaps == nil && res.PythonMaps == nil {
		return res, fmt.Errorf("no python found")
	}
	return res, nil
}
