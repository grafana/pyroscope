package symtab

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/grafana/pyroscope/ebpf/symtab/elf"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

type ProcTable struct {
	logger     log.Logger
	ranges     []elfRange
	file2Table map[file]*ElfTable
	options    ProcTableOptions
	rootFS     string
	err        error
}

type ProcTableDebugInfo struct {
	ElfTables     map[string]elf.SymTabDebugInfo `river:"elfs,block,optional"`
	Size          int                            `river:"size,attr,optional"`
	Pid           int                            `river:"pid,attr,optional"`
	LastUsedRound int                            `river:"last_used_round,attr,optional"`
}

func (p *ProcTable) DebugInfo() ProcTableDebugInfo {
	res := ProcTableDebugInfo{
		Pid:       p.options.Pid,
		Size:      len(p.file2Table),
		ElfTables: make(map[string]elf.SymTabDebugInfo),
	}
	for f, e := range p.file2Table {
		d := e.table.DebugInfo()
		if d.Size != 0 {
			res.ElfTables[fmt.Sprintf("%x %x %s", f.dev, f.inode, f.path)] = d
		}
	}
	return res
}

type ProcTableOptions struct {
	Pid int
	ElfTableOptions
}

func NewProcTable(logger log.Logger, options ProcTableOptions) *ProcTable {
	return &ProcTable{
		logger:     logger,
		file2Table: make(map[file]*ElfTable),
		options:    options,
		rootFS:     path.Join("/proc", strconv.Itoa(options.Pid), "root"),
	}
}

type elfRange struct {
	mapRange *ProcMap
	// may be nil
	elfTable *ElfTable
}

func (p *ProcTable) Refresh() {
	if p.err != nil {
		return
	}
	procMaps, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", p.options.Pid))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			level.Error(p.logger).Log("msg", "failed to read /proc/pid/maps", "err", err)
		}
		p.options.Metrics.ProcErrors.WithLabelValues(errorType(err)).Inc()
		p.err = err
		return
	}
	p.err = p.refreshProcMap(procMaps)
	if p.err != nil {
		_ = level.Error(p.logger).Log("err", p.err)
	} else {
	}
}

func (p *ProcTable) Error() error {
	return p.err
}

func (p *ProcTable) refreshProcMap(procMaps []byte) error {
	// todo support perf map files
	for i := range p.ranges {
		p.ranges[i].elfTable = nil
	}
	p.ranges = p.ranges[:0]
	filesToKeep := make(map[file]struct{})
	maps, err := ParseProcMapsExecutableModules(procMaps, true)
	if err != nil {
		return err
	}

	for _, m := range maps {
		p.ranges = append(p.ranges, elfRange{
			mapRange: m,
		})
		r := &p.ranges[len(p.ranges)-1]
		e := p.getElfTable(r)
		if e != nil {
			r.elfTable = e
			filesToKeep[r.mapRange.file()] = struct{}{}
		}
	}
	var filesToDelete []file
	for f := range p.file2Table {
		_, keep := filesToKeep[f]
		if !keep {
			filesToDelete = append(filesToDelete, f)
		}
	}
	for _, f := range filesToDelete {
		delete(p.file2Table, f)
	}
	return nil
}

func (p *ProcTable) getElfTable(r *elfRange) *ElfTable {
	f := r.mapRange.file()
	e, ok := p.file2Table[f]
	if !ok {
		e = p.createElfTable(r.mapRange)
		if e != nil {
			p.file2Table[f] = e
		}
	}
	return e
}

func (p *ProcTable) Resolve(pc uint64) Symbol {
	if pc == 0xcccccccccccccccc || pc == 0x9090909090909090 {
		return Symbol{Start: 0, Name: "end_of_stack", Module: "[unknown]"}
	}
	i, found := slices.BinarySearchFunc(p.ranges, pc, binarySearchElfRange)
	if !found {
		return Symbol{}
	}
	r := p.ranges[i]
	t := r.elfTable
	if t == nil {
		return Symbol{}
	}
	s := t.Resolve(pc)
	moduleOffset := pc - t.base
	if s == "" {
		return Symbol{Start: moduleOffset, Module: r.mapRange.Pathname}
	}

	return Symbol{Start: moduleOffset, Name: s, Module: r.mapRange.Pathname}
}

func (p *ProcTable) createElfTable(m *ProcMap) *ElfTable {
	if !strings.HasPrefix(m.Pathname, "/") {
		return nil
	}
	e := NewElfTable(p.logger, m, p.rootFS, m.Pathname, p.options.ElfTableOptions)
	return e
}

func (p *ProcTable) Cleanup() {
	for _, table := range p.file2Table {
		table.Cleanup()
	}
}

func (p *ProcTable) Pid() int {
	return p.options.Pid
}

func binarySearchElfRange(e elfRange, pc uint64) int {
	if pc < e.mapRange.StartAddr {
		return 1
	}
	if pc >= e.mapRange.EndAddr {
		return -1
	}
	return 0
}
