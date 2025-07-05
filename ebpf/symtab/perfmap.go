package symtab

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

type PerfMap struct {
	symbols []Symbol
}

func NewPerfMap(pid int) (*PerfMap, error) {
	nsPid := getNsPid(pid)

	perfMapPath := fmt.Sprintf("/proc/%d/root/tmp/perf-%d.map", pid, nsPid)
	f, err := os.Open(perfMapPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	pm := &PerfMap{}
	if pm.symbols, err = parsePerfMap(f); err != nil {
		return nil, err
	}
	return pm, nil
}

func (pm *PerfMap) Resolve(pc uint64) Symbol {
	i, ok := slices.BinarySearchFunc(pm.symbols, -1, func(s Symbol, _ int) int {
		if s.Start <= pc {
			if pc < s.Start+s.size {
				return 0
			}
			return -1
		}
		return 1
	})
	if ok {
		return pm.symbols[i]
	}
	return Symbol{}
}

func parsePerfMap(reader io.Reader) ([]Symbol, error) {
	var (
		syms []Symbol
		err  error
	)
	scanner := bufio.NewScanner(reader)
	for i := 0; scanner.Scan(); i++ {
		line := scanner.Text()
		items := strings.SplitN(line, " ", 3)
		if len(items) != 3 {
			return nil, fmt.Errorf("invalid line (does not contain 3 parts): %s", line)
		}
		s := Symbol{
			Name:       items[2],
			generation: i,
		}
		s.Start, err = strconv.ParseUint(strings.TrimPrefix(items[0], "0x"), 16, 64)
		if err != nil {
			return nil, err
		}
		s.size, err = strconv.ParseUint(strings.TrimPrefix(items[1], "0x"), 16, 64)
		if err != nil {
			return nil, err
		}
		syms = append(syms, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(syms, func(a, b Symbol) int {
		return cmp.Compare(a.Start, b.Start)
	})
	syms = removeOverlaps(syms)
	return syms, nil
}

// Node.js appends new symbols to the perf map file, and their addresses can overlap earlier ones.
// This function removes overlapping symbols, keeping those that appeared latest.
func removeOverlaps(syms []Symbol) []Symbol {
	if len(syms) == 0 {
		return nil
	}

	var (
		lastValid = 0
		toRemove  = make(map[int]struct{})
	)

	for i := 1; i < len(syms); i++ {
		prev := syms[lastValid]
		curr := syms[i]

		if prev.Start+prev.size > curr.Start {
			// Overlap detected
			if prev.generation > curr.generation {
				toRemove[i] = struct{}{}
			} else {
				toRemove[lastValid] = struct{}{}
				lastValid = i
			}
		} else {
			lastValid = i
		}
	}

	// Compact the slice in-place
	write := 0
	for i := range syms {
		if _, skip := toRemove[i]; !skip {
			syms[write] = syms[i]
			write++
		}
	}

	return syms[:write]
}

func getNsPid(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return pid
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "NSpid:" {
			if len(fields) == 3 {
				if nsPid, err := strconv.ParseUint(fields[2], 10, 32); err == nil {
					return int(nsPid)
				}
			}
			return pid
		}
	}
	return pid
}
