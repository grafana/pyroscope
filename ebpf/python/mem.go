package python

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
	"unsafe"

	"github.com/cilium/ebpf/link"
	"github.com/grafana/pyroscope/ebpf/symtab"
)

// typedef struct {
// void *ctx;
//
// void *(*malloc)(void *ctx, size_t size);
//
// void *(*calloc)(void *ctx, size_t nelem, size_t elsize);
//
// void *(*realloc)(void *ctx, void *ptr, size_t new_size);
//
// void (*free)(void *ctx, void *ptr);
// } PyMemAllocatorEx;
// void ebpf_assist_trap(size_t size) {
type PyMemAllocatorEx struct {
	ctx     uint64
	malloc  uint64
	calloc  uint64
	realloc uint64
	free    uint64
}

func (p *PyMemAllocatorEx) String() string {
	return fmt.Sprintf("ctx %x malloc %x calloc %x realloc %x free %x", p.ctx, p.malloc, p.calloc, p.realloc, p.free)
}
func (p *PyMemAllocatorEx) slice() []byte {
	return (*[unsafe.Sizeof(*p)]byte)(unsafe.Pointer(p))[:]
}

func (p *PyMemAllocatorEx) read(fd *os.File, at int64) error {
	_, err := fd.Seek(at, io.SeekStart)
	if err != nil {
		return err
	}
	_, err = fd.Read(p.slice())
	return err
}

func (p *PyMemAllocatorEx) write(fd *os.File, at int64) error {
	_, err := fd.Seek(at, io.SeekStart)
	if err != nil {
		return err
	}
	_, err = fd.Write(p.slice())
	return err
}

func (s *Perf) InitMemSampling(data *ProcData) error {
	process := s.pids[uint32(data.PID)]
	if process == nil {
		return fmt.Errorf("no process %d", data.PID)
	}

	pi := data.ProcInfo
	if pi.PyMemSampler == nil {
		return nil
	}
	samplerAddr := func(off int64) int64 {
		return int64(pi.PyMemSampler[0].StartAddr) + off
	}

	ef, err := elf.Open(fmt.Sprintf("/proc/%d/root%s", data.PID, pi.PyMemSampler[0].Pathname))
	if err != nil {
		return err
	}
	defer ef.Close()

	vm, err := newVM(data.PID)
	if err != nil {
		return fmt.Errorf("open mem %w", err)
	}
	defer vm.Close()

	symbols, err := ef.DynamicSymbols()
	if err != nil {
		return err
	}
	var (
		ebpf_assist_delegate_allocator int64
		ebpf_assist_trap_ptr           int64
		ebpf_assist_trap               int64
		ebpf_assist_sampling_allocator int64
	)

	for _, s := range symbols {
		switch s.Name {
		case "ebpf_assist_trap_ptr":
			ebpf_assist_trap_ptr = int64(s.Value)
		case "ebpf_assist_delegate_allocator":
			ebpf_assist_delegate_allocator = int64(s.Value)
		case "ebpf_assist_sampling_allocator":
			ebpf_assist_sampling_allocator = int64(s.Value)
		}
	}
	if ebpf_assist_delegate_allocator == 0 || ebpf_assist_trap_ptr == 0 || ebpf_assist_sampling_allocator == 0 {
		return fmt.Errorf("wrong lib %x %x %x", ebpf_assist_delegate_allocator, ebpf_assist_trap_ptr, ebpf_assist_sampling_allocator)
	}
	if err = readVM(vm, &ebpf_assist_trap, samplerAddr(ebpf_assist_trap_ptr)); err != nil {
		return fmt.Errorf("read ebpf_assist_trap_ptr %w", err)
	}
	if !pointsTo(uint64(ebpf_assist_trap), pi.PyMemSampler) {
		return fmt.Errorf("ebpf_assist_trap points to unknown memory %x %+v", ebpf_assist_trap, pi.PyMemSampler)
	}
	var offsetAssistTrap = uint64(ebpf_assist_trap) - pi.PyMemSampler[0].StartAddr
	// hardcoded, TODO find it dynamically
	var offsetPyObjectALlocator = int64(0x7f7c5a070280 - 0x7f7c59c00000)
	var addrPyObjectALlocator = int64(data.Base.StartAddr) + offsetPyObjectALlocator
	var addrAssistDelegateAllocator = samplerAddr(ebpf_assist_delegate_allocator)
	var addrAssistSamplingAllocator = samplerAddr(ebpf_assist_sampling_allocator)

	s.logger.Log("op", "pymem sampling", "offsetAssistTrap", fmt.Sprintf("0x%x", offsetAssistTrap))
	s.logger.Log("op", "pymem sampling", "base", fmt.Sprintf("0x%x", data.Base.StartAddr))
	s.logger.Log("op", "pymem sampling", "alloc", fmt.Sprintf("0x%x", addrPyObjectALlocator))
	s.logger.Log("op", "pymem sampling", "assist delegate allocator ", fmt.Sprintf("0x%x", addrAssistDelegateAllocator))
	s.logger.Log("op", "pymem sampling", "trap  ", fmt.Sprintf("0x%x 0x%x", ebpf_assist_trap_ptr, ebpf_assist_trap))
	s.logger.Log("op", "pymem sampling", "pymemsampler  ", symtab.ProcMaps(pi.PyMemSampler).String())

	allocator := new(PyMemAllocatorEx)

	if err = readVM(vm, allocator, addrPyObjectALlocator); err != nil {
		return fmt.Errorf("read current allocator %w", err)
	}
	s.logger.Log("op", "pymem sampling", "allocator", allocator.String())
	if allocatorPointsTo(allocator, getPythonMaps(pi)) {
		//TODO  stop app while writing
		if err = writeVM(vm, allocator, addrAssistDelegateAllocator); err != nil {
			return fmt.Errorf("write current allocator to pysampler %w", err)
		}
		if err = readVM(vm, allocator, addrAssistSamplingAllocator); err != nil {
			return fmt.Errorf("read current allocator from pysampler %w", err)
		}
		if err = writeVM(vm, allocator, addrPyObjectALlocator); err != nil {
			return fmt.Errorf("write current allocator to pysampler %w", err)
		}
	} else if allocatorPointsTo(allocator, pi.PyMemSampler) {
		// profiler restart
	} else {
		return fmt.Errorf("allocator points to unknown memory %s expected %s", allocator.String(),
			symtab.ProcMaps(pi.PyMemSampler).String())
	}
	var trapInstructions uint64
	if err = readVM(vm, &trapInstructions, ebpf_assist_trap); err != nil {
		return fmt.Errorf("read trap instruction %w", err)
	}
	//if (trapInstructions & 0xff) != 0xc3 { // sanity check
	//	return fmt.Errorf("wrong trap instruction %x", trapInstructions)
	//}
	exe, err := link.OpenExecutable(pi.PyMemSampler[0].Pathname)
	if err != nil {
		return fmt.Errorf("opening %q executable file: %w", pi.PyMemSampler[0].Pathname, err)
	}
	if s.memProg == nil {
		return fmt.Errorf("no mem prog")
	}
	up, err := exe.Uprobe("", s.memProg, &link.UprobeOptions{
		Address: offsetAssistTrap,
		PID:     data.PID,
	})
	if err != nil {
		return fmt.Errorf("setting uprobe: %w", err)
	}
	process.memSamplingLink = up
	return nil
}

func allocatorPointsTo(a *PyMemAllocatorEx, mem []*symtab.ProcMap) bool {
	return a.ctx == 0 &&
		pointsTo(a.malloc, mem) &&
		pointsTo(a.calloc, mem) &&
		pointsTo(a.realloc, mem) &&
		pointsTo(a.free, mem)
}

func pointsTo(p uint64, mem []*symtab.ProcMap) bool {
	for _, procMap := range mem {
		if p >= procMap.StartAddr && p < procMap.EndAddr {
			return true
		}
	}
	return false
}

type mem struct {
	pid int
	*os.File
}

func (m *mem) u64() (uint64, error) {
	var res uint64
	err := readVM(m, &res, 0)
	return res, err
}
func (m *mem) Close() error {
	return m.File.Close()
}

func newVM(pid int) (*mem, error) {
	f, err := os.OpenFile(fmt.Sprintf("/proc/%d/mem", pid), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &mem{pid: pid, File: f}, nil
}

func readVM[T any](v *mem, p *T, at int64) error {
	slice := unsafe.Slice((*byte)(unsafe.Pointer(p)), unsafe.Sizeof(*p))

	_, err := v.Seek(int64(at), io.SeekStart)
	if err != nil {
		return nil
	}
	_, err = v.Read(slice)
	return err
}

func writeVM[T any](v *mem, p *T, at int64) error {
	slice := unsafe.Slice((*byte)(unsafe.Pointer(p)), unsafe.Sizeof(*p))
	_, err := v.Seek(int64(at), io.SeekStart)
	if err != nil {
		return nil
	}
	_, err = v.Write(slice)
	return err
}
