package symtab

import (
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/metrics"
	elf2 "github.com/grafana/pyroscope/ebpf/symtab/elf"
	"github.com/ianlancetaylor/demangle"
)

var (
	errElfBaseNotFound = fmt.Errorf("elf base not found")
)

type ElfTable struct {
	fs          string
	elfFilePath string
	table       SymbolNameResolver
	base        uint64

	loaded       bool
	loadedCached bool
	err          error

	options ElfTableOptions
	logger  log.Logger
	procMap *ProcMap
}
type SymbolOptions struct {
	GoTableFallback    bool
	PythonFullFilePath bool
	DemangleOptions    []demangle.Option
}

var DefaultSymbolOptions = &SymbolOptions{
	GoTableFallback: false,
}

type ElfTableOptions struct {
	ElfCache      *ElfCache
	Metrics       *metrics.SymtabMetrics
	SymbolOptions *SymbolOptions
}

func NewElfTable(logger log.Logger, procMap *ProcMap, fs string, elfFilePath string, options ElfTableOptions) *ElfTable {
	if options.SymbolOptions == nil {
		options.SymbolOptions = DefaultSymbolOptions
	}
	if options.Metrics == nil {
		panic("metrics is nil")
	}
	res := &ElfTable{
		procMap:     procMap,
		fs:          fs,
		elfFilePath: elfFilePath,
		logger:      logger,
		options:     options,
		table:       &noopSymbolNameResolver{},
	}
	return res
}

func (et *ElfTable) findBase(e *elf2.MMapedElfFile) bool {
	m := et.procMap
	if e.FileHeader.Type == elf.ET_EXEC {
		et.base = 0
		return true
	}
	for _, prog := range e.Progs {
		if prog.Type == elf.PT_LOAD && (prog.Flags&elf.PF_X != 0) {
			if uint64(m.Offset) == prog.Off {
				et.base = m.StartAddr - prog.Vaddr
				return true
			}
			alignedProgOffset := uint64(prog.Off) & 0xfffffffffffff000
			if uint64(m.Offset) == alignedProgOffset {
				d := prog.Off - alignedProgOffset
				et.base = m.StartAddr + d - prog.Vaddr
				return true
			}
		}
	}
	return false
}

func (et *ElfTable) load() {
	if et.loaded {
		return
	}
	et.loaded = true
	fsElfFilePath := path.Join(et.fs, et.elfFilePath)

	me, err := elf2.NewMMapedElfFile(fsElfFilePath)
	if err != nil {
		et.onLoadError(err)
		return
	}
	defer me.Close() // todo do not close if it is the selected elf

	if !et.findBase(me) {
		et.onLoadError(errElfBaseNotFound)
		return
	}
	buildID, err := me.BuildID()
	if err != nil {
		level.Error(et.logger).Log("msg", "failed to get build id", "err", err, "f", et.elfFilePath, "fs", et.fs)
	}

	symbols := et.options.ElfCache.GetSymbolsByBuildID(buildID)
	if symbols != nil {
		et.table = symbols
		et.loadedCached = true
		return
	}
	fileInfo, err := os.Stat(fsElfFilePath)
	if err != nil {
		et.onLoadError(err)
		return
	}
	symbols = et.options.ElfCache.GetSymbolsByStat(statFromFileInfo(fileInfo))
	if symbols != nil {
		et.table = symbols
		et.loadedCached = true
		return
	}

	debugFilePath := et.findDebugFile(buildID, me)
	if debugFilePath != "" {
		debugMe, err := elf2.NewMMapedElfFile(path.Join(et.fs, debugFilePath))
		if err != nil {
			et.onLoadError(err)
			return
		}
		defer debugMe.Close() // todo do not close if it is the selected elf

		symbols, err = et.createSymbolTable(debugMe)
		if err != nil {
			et.onLoadError(err)
			return
		}
		et.table = symbols
		et.options.ElfCache.CacheByBuildID(buildID, symbols)
		return
	}

	symbols, err = et.createSymbolTable(me)
	if err != nil {
		et.onLoadError(err)
		return
	}

	et.table = symbols
	if buildID.Empty() {
		et.options.ElfCache.CacheByStat(statFromFileInfo(fileInfo), symbols)
	} else {
		et.options.ElfCache.CacheByBuildID(buildID, symbols)
	}
}

func (et *ElfTable) createSymbolTable(me *elf2.MMapedElfFile) (SymbolNameResolver, error) {
	level.Debug(et.logger).Log("msg", "create symbol table", "path", me.FilePath())
	goTable, goErr := me.NewGoTable()
	if !et.options.SymbolOptions.GoTableFallback && goErr == nil {
		return goTable, nil
	}
	symbolOptions := elf2.SymbolsOptions{
		DemangleOptions: et.options.SymbolOptions.DemangleOptions,
	}
	if goErr == nil && goTable.Index.Entry.Length() > 0 {
		symbolOptions.FilterFrom = goTable.Index.Entry.Get(0)
		symbolOptions.FilterTo = goTable.Index.End
	}
	origSymTable, origErr := me.NewSymbolTable(&symbolOptions)

	var symTable elf2.SymbolTableInterface
	var symErr error
	if origErr == nil && origSymTable.HasSection(elf.SHT_SYMTAB) {
		symTable = origSymTable
		symErr = nil
	} else {
		miniSymTable, miniErr := me.NewMiniDebugInfoSymbolTable(&symbolOptions)
		if origErr != nil && miniErr != nil {
			symTable = nil
			symErr = fmt.Errorf("o: %s m: %s", origErr.Error(), miniErr.Error())
		} else {
			tab := &elf2.SymbolTableWithMiniDebugInfo{
				Primary:   origSymTable,
				MiniDebug: miniSymTable,
			}
			symTable = tab
			symErr = nil
		}
	}

	if symErr != nil && goErr != nil {
		return nil, fmt.Errorf("s: {%s} g: {%s}", symErr.Error(), goErr.Error())
	}
	if symErr == nil && goErr == nil {
		return &elf2.GoTableWithFallback{
			GoTable:  goTable,
			SymTable: symTable,
		}, nil
	}
	if symErr == nil {
		return symTable, nil
	}
	if goTable != nil {
		return goTable, nil
	}
	panic("unreachable")
}

var errTableDead = fmt.Errorf("non cached table dead")

func (et *ElfTable) Resolve(pc uint64) string {
	if !et.loaded {
		et.load()
	}
	if et.err != nil {
		return ""
	}
	pc -= et.base
	res := et.table.Resolve(pc)
	if res != "" {
		return res
	}
	if !et.table.IsDead() {
		return ""
	}
	if !et.loadedCached {
		et.err = errTableDead
		return ""
	}
	et.table = &noopSymbolNameResolver{}
	et.loaded = false
	et.loadedCached = false
	et.load()
	if et.err != nil {
		return res
	}
	return et.table.Resolve(pc)
}

func (et *ElfTable) Cleanup() {
	if et.table != nil {
		et.table.Cleanup()
	}
}

func (et *ElfTable) findDebugFileWithBuildID(buildID elf2.BuildID) string {
	id := buildID.ID
	if len(id) < 3 || !buildID.GNU() {
		return ""
	}

	debugFile := fmt.Sprintf("/usr/lib/debug/.build-id/%s/%s.debug", id[:2], id[2:])
	fsDebugFile := path.Join(et.fs, debugFile)
	_, err := os.Stat(fsDebugFile)
	if err == nil {
		return debugFile
	}

	return ""
}

func (et *ElfTable) findDebugFile(buildID elf2.BuildID, elfFile *elf2.MMapedElfFile) string {
	// https://sourceware.org/gdb/onlinedocs/gdb/Separate-Debug-Files.html
	// So, for example, suppose you ask GDB to debug /usr/bin/ls, which has a debug link that specifies the file
	// ls.debug, and a build ID whose value in hex is abcdef1234. If the list of the global debug directories
	// includes /usr/lib/debug, then GDB will look for the following debug information files, in the indicated order:
	//
	//- /usr/lib/debug/.build-id/ab/cdef1234.debug
	//- /usr/bin/ls.debug
	//- /usr/bin/.debug/ls.debug
	//- /usr/lib/debug/usr/bin/ls.debug.
	debugFile := et.findDebugFileWithBuildID(buildID)
	if debugFile != "" {
		return debugFile
	}
	debugFile = et.findDebugFileWithDebugLink(elfFile)
	return debugFile
}

func (et *ElfTable) findDebugFileWithDebugLink(elfFile *elf2.MMapedElfFile) string {
	fs := et.fs
	elfFilePath := et.elfFilePath
	debugLinkSection := elfFile.Section(".gnu_debuglink")
	if debugLinkSection == nil {
		return ""
	}
	data, err := elfFile.SectionData(debugLinkSection)
	if err != nil {
		return ""
	}
	if len(data) < 6 {
		return ""
	}
	crc := data[len(data)-4:]
	_ = crc
	debugLink := cString(data)

	// /usr/bin/ls.debug
	fsDebugFile := path.Join(path.Dir(elfFilePath), debugLink)
	_, err = os.Stat(path.Join(fs, fsDebugFile))
	if err == nil {
		return fsDebugFile
	}
	// /usr/bin/.debug/ls.debug
	fsDebugFile = path.Join(path.Dir(elfFilePath), ".debug", debugLink)
	_, err = os.Stat(path.Join(fs, fsDebugFile))
	if err == nil {
		return fsDebugFile
	}
	// /usr/lib/debug/usr/bin/ls.debug.
	fsDebugFile = path.Join("/usr/lib/debug", path.Dir(elfFilePath), debugLink)
	_, err = os.Stat(path.Join(fs, fsDebugFile))
	if err == nil {
		return fsDebugFile
	}

	return ""
}

func cString(bs []byte) string {
	i := 0
	for ; i < len(bs); i++ {
		if bs[i] == 0 {
			break
		}
	}
	return string(bs[:i])
}

type ElfDebugInfo struct {
	SymbolsCount int    `alloy:"symbols_count,attr,optional" river:"symbols_count,attr,optional"`
	File         string `alloy:"file,attr,optional" river:"file,attr,optional"`
}

func (et *ElfTable) DebugInfo() elf2.SymTabDebugInfo {
	return et.table.DebugInfo()
}

func (et *ElfTable) onLoadError(err error) {
	et.err = err
	var l log.Logger
	if errors.Is(err, os.ErrNotExist) {
		l = level.Debug(et.logger)
	} else {
		l = level.Error(et.logger)
	}
	l.Log(
		"msg", "failed to load elf table",
		"err", et.err,
		"f", et.elfFilePath,
		"fs", et.fs)
	if et.options.Metrics != nil {
		et.options.Metrics.ElfErrors.WithLabelValues(errorType(et.err)).Inc()
	}
}

func errorType(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "ErrNotExist"
	}
	if errors.Is(err, os.ErrPermission) {
		return "ErrPermission"
	}
	if errors.Is(err, os.ErrClosed) {
		return "ErrClosed"
	}
	if errors.Is(err, os.ErrInvalid) {
		return "ErrInvalid"
	}
	if errors.Is(err, errElfBaseNotFound) {
		return "ElfBaseNotFound"
	}
	return "Other"
}
