package symtab

import (
	"debug/elf"
	"fmt"
	"golang.org/x/exp/slices"
	"os"
	"path"
	"strconv"
	"strings"
)

type ProcTable struct {
	ranges     []elfRange
	file2Table map[string]*ElfTable
	options    ProcTableOptions
	rootFS     string
}

type ProcTableOptions struct {
	Pid              int
	IgnoreDebugFiles bool
}

func NewProcTable(options ProcTableOptions) *ProcTable {
	return &ProcTable{
		file2Table: make(map[string]*ElfTable),
		options:    options,
		rootFS:     path.Join("/proc", strconv.Itoa(options.Pid), "root"),
	}
}

type elfRange struct {
	mapRange procMapEntry
	elfTable *ElfTable
}

func (p *ProcTable) Refresh() {
	procMaps, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", p.options.Pid))
	if err != nil {
		return // todo return err
	}
	p.refresh(procMaps)
}

func (p *ProcTable) refresh(procMaps []byte) {
	// todo perf map files
	// todo do share elf table between pids, just create a copy with a different base, make sure to check inode
	// todo remove ElfTables which are no longer in mappings ranges
	for i := range p.ranges {
		p.ranges[i].elfTable = nil
	}
	p.ranges = p.ranges[:0]

	maps, err := parseProcMaps(procMaps)
	if err != nil {
		return
	}
	for _, m := range maps {
		p.ranges = append(p.ranges, elfRange{
			mapRange: m,
		})
		r := &p.ranges[len(p.ranges)-1]
		e := p.getElfTable(r)
		if e != nil {
			r.elfTable = e
		}

	}
}

func (p *ProcTable) getElfTable(r *elfRange) *ElfTable {
	e, ok := p.file2Table[r.mapRange.file]
	if !ok {
		e = p.createElfTable(r)
		//println("create elftable", e)
		p.file2Table[r.mapRange.file] = e
	}
	return e
}

func (p *ProcTable) Resolve(pc uint64) *Symbol {
	i, found := slices.BinarySearchFunc(p.ranges, pc, binarySearchElfRange)
	if !found {
		return nil
	}
	t := p.ranges[i].elfTable
	if t == nil {
		return nil
	}
	sym := t.table.Resolve(pc)
	return sym
}

func (p *ProcTable) Close() {

}

func (p *ProcTable) createElfTable(m *elfRange) *ElfTable {
	if !strings.HasPrefix(m.mapRange.file, "/") {
		//println("skip", m.mapRange.file)
		return nil
	}
	file := m.mapRange.file
	e, err := NewElfTable(p.rootFS, file, !p.options.IgnoreDebugFiles)
	if err != nil {
		println("NewElfTable err", err.Error())
		return nil
	}

	if rebase(m, e) {
		return e
	} else {
		return nil
	}
}

func rebase(m *elfRange, e *ElfTable) bool {

	if e.typ == elf.ET_EXEC {
		return true
	}
	for _, executable := range e.executables {
		if m.mapRange.offset == executable.Off {
			base := m.mapRange.start - executable.Vaddr
			//fmt.Printf("base %x %s\n", base, m.file)

			e.table.Rebase(base)
			return true
		}
	}
	//println("base not found")
	return false
}

func binarySearchElfRange(e elfRange, pc uint64) int {
	if pc < e.mapRange.start {
		return 1
	}
	if pc >= e.mapRange.end {
		return -1
	}
	return 0
}
