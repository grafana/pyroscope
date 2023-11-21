package python

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"

	log2 "github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

var reMuslVersion = regexp.MustCompile("1\\.([12])\\.(\\d+)\\D")

func GetMuslVersionFromFile(f string) (Version, error) {
	muslFD, err := os.Open(f)
	if err != nil {
		return Version{}, fmt.Errorf("couldnot determine musl version %s %w", f, err)
	}
	defer muslFD.Close()
	return GetMuslVersionFromReader(muslFD)
}

// GetMuslVersionFromReader return minor musl version. For example 1 for 1.1.44 and 2 for 1.2.4
func GetMuslVersionFromReader(r io.Reader) (Version, error) {
	m, err := rgrep(r, reMuslVersion)
	if err != nil {
		return Version{}, fmt.Errorf("rgrep musl version  %w", err)
	}
	minor, _ := strconv.Atoi(string(m[1]))
	patch, _ := strconv.Atoi(string(m[2]))
	return Version{Major: 1, Minor: minor, Patch: patch}, nil
}

var reGlibcVersion = regexp.MustCompile("glibc 2\\.(\\d+)\\D")

func GetGlibcVersionFromFile(f string) (Version, error) {
	muslFD, err := os.Open(f)
	if err != nil {
		return Version{}, fmt.Errorf("couldnot determine glibc version %s %w", f, err)
	}
	defer muslFD.Close()
	return GetGlibcVersionFromReader(muslFD)
}

func GetGlibcVersionFromReader(r io.Reader) (Version, error) {
	m, err := rgrep(r, reGlibcVersion)
	if err != nil {
		return Version{}, fmt.Errorf("rgrep python version  %w", err)
	}
	minor, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return Version{}, fmt.Errorf("error trying to grep musl minor version %s, %w", string(m[0]), err)
	}

	return Version{Major: 2, Minor: minor}, nil
}

func GetGlibcOffsets(version Version) (PerfLibc, bool, error) {
	offsets, ok := glibcOffsets[version]
	if ok {
		return PerfLibc{
			Musl:                    false,
			PthreadSize:             offsets.PthreadSize,
			PthreadSpecific1stblock: offsets.PthreadSpecific1stblock,
		}, false, nil
	}
	versions := make([]Version, 0, len(glibcOffsets))
	for v := range glibcOffsets {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Compare(&versions[j]) < 0
	})
	if version.Compare(&versions[len(versions)-1]) > 0 {
		return PerfLibc{
			Musl:                    false,
			PthreadSize:             glibcOffsets[versions[len(versions)-1]].PthreadSize,
			PthreadSpecific1stblock: glibcOffsets[versions[len(versions)-1]].PthreadSpecific1stblock,
		}, true, nil
	}
	return PerfLibc{}, false, fmt.Errorf("unsupported glibc version %v", version)
}

func GetLibc(l log2.Logger, pid uint32, info ProcInfo) (PerfLibc, error) {
	if info.Glibc == nil && info.Musl == nil {
		return PerfLibc{}, fmt.Errorf("could not determine libc version %d, no libc found", pid)
	}
	if info.Musl != nil {
		muslPath := fmt.Sprintf("/proc/%d/root%s", pid, info.Musl[0].Pathname)
		muslVersion, err := GetMuslVersionFromFile(muslPath)
		if err != nil {
			return PerfLibc{}, fmt.Errorf("couldnot determine musl version %s %w", muslPath, err)
		}
		res := PerfLibc{Musl: true}
		mo, guess, err := getVersionGuessing(muslVersion, muslOffsets)
		if err != nil {
			return PerfLibc{}, fmt.Errorf("unsupported musl version %w %+v", err, muslVersion)
		}
		if guess {
			_ = level.Warn(l).Log("msg", "musl offsets were not found, but guessed from the closest version")
		}
		res.PthreadSize = mo.PthreadSize
		res.PthreadSpecific1stblock = mo.PthreadTsd
		_ = level.Debug(l).Log("msg", "musl offsets", "offsets", fmt.Sprintf("%+v", res))
		return res, nil
	}

	glibcPath := fmt.Sprintf("/proc/%d/root%s", pid, info.Glibc[0].Pathname)
	glibcVersion, err := GetGlibcVersionFromFile(glibcPath)
	if err != nil {
		return PerfLibc{}, fmt.Errorf("couldnot determine glibc version %s %w", glibcPath, err)
	}

	res, guess, err := GetGlibcOffsets(glibcVersion)
	if err != nil {
		return PerfLibc{}, fmt.Errorf("unsupported glibc version %w %+v", err, glibcVersion)
	}
	if guess {
		_ = level.Warn(l).Log("msg", "glibc offsets were not found, but guessed from the closest version")
	}
	_ = level.Debug(l).Log("msg", "glibc offsets", "offsets", fmt.Sprintf("%+v", res))
	return res, nil

}
