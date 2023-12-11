package python

import (
	"bytes"
	"debug/elf"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/grafana/pyroscope/ebpf/symtab"
	"golang.org/x/arch/x86/x86asm"
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
	fmt.Println("InitMemSampling")
	if data.ProcInfo.Version.Minor != 11 {
		return fmt.Errorf("mem sampling nt implemented %+v", data.ProcInfo.Version)
	}
	process := s.pids[uint32(data.PID)]
	if process == nil {
		return fmt.Errorf("no process %d", data.PID)
	}

	//pi := data.ProcInfo

	vm, err := newVM(data.PID)
	if err != nil {
		return fmt.Errorf("open mem %w", err)
	}
	defer vm.Close()

	addrPyObjectAllocator := getPyObjectAllocatorAddress(vm, data)
	if addrPyObjectAllocator == 0 {
		return fmt.Errorf("could not find PyObject allocator address")
	}
	// hardcoded, TODO find it dynamically
	//var offsetPyObjectALlocator = int64(0x7f7c5a070280 - 0x7f7c59c00000)
	//var addrPyObjectAllocator = int64(data.Base.StartAddr) + offsetPyObjectALlocator
	s.logger.Log("op", "pymem sampling", "base", fmt.Sprintf("0x%x", data.Base.StartAddr))
	s.logger.Log("op", "pymem sampling", "alloc", fmt.Sprintf("0x%x", addrPyObjectAllocator))

	allocator := new(PyMemAllocatorEx)
	if err = readVM(vm, allocator, addrPyObjectAllocator); err != nil {
		return fmt.Errorf("read current allocator %w", err)
	}
	if !allocatorPointsTo(allocator, getPythonMaps(data.ProcInfo)) {
		return fmt.Errorf("unknown allocator %+v", allocator)
	}

	objAllocatorFreeAddr := addrPyObjectAllocator + 0x20
	hookFree := func(to int64) error {
		if err = writeVM(vm, &to, objAllocatorFreeAddr); err != nil {
			return fmt.Errorf("write injector free hook %w", err)
		}
		return nil
	}
	_ = hookFree
	//_ = hookFree(0xcafebabe)

	injector, err := getInjector(data, int64(allocator.free), objAllocatorFreeAddr, "/huihui")
	if err != nil {
		return err
	}
	fmt.Printf("injector %s\n", hex.EncodeToString(injector))

	injectorAddress, err := writeInjector(vm, data, injector)
	if err != nil {
		return fmt.Errorf("write injector failed: %w", err)
	}
	fmt.Printf("injector written at %x\n", injectorAddress)

	if err := hookFree(0xcafebabe00); err != nil {
		return fmt.Errorf("could not hook allocator.free to injector")
	}
	fmt.Println("injector hooked")

	return nil
	//if pi.PyMemSampler != nil {
	//	return fmt.Errorf("not implemented TODO")
	//}
	//samplerAddr := func(off int64) int64 {
	//	return int64(pi.PyMemSampler[0].StartAddr) + off
	//}
	//
	//ef, err := elf.Open(fmt.Sprintf("/proc/%d/root%s", data.PID, pi.PyMemSampler[0].Pathname))
	//if err != nil {
	//	return err
	//}
	//defer ef.Close()
	//
	//symbols, err := ef.DynamicSymbols()
	//if err != nil {
	//	return err
	//}
	//var (
	//	ebpf_assist_delegate_allocator int64
	//	ebpf_assist_trap_ptr           int64
	//	ebpf_assist_trap               int64
	//	ebpf_assist_sampling_allocator int64
	//)
	//
	//for _, s := range symbols {
	//	switch s.Name {
	//	case "ebpf_assist_trap_ptr":
	//		ebpf_assist_trap_ptr = int64(s.Value)
	//	case "ebpf_assist_delegate_allocator":
	//		ebpf_assist_delegate_allocator = int64(s.Value)
	//	case "ebpf_assist_sampling_allocator":
	//		ebpf_assist_sampling_allocator = int64(s.Value)
	//	}
	//}
	//if ebpf_assist_delegate_allocator == 0 || ebpf_assist_trap_ptr == 0 || ebpf_assist_sampling_allocator == 0 {
	//	return fmt.Errorf("wrong lib %x %x %x", ebpf_assist_delegate_allocator, ebpf_assist_trap_ptr, ebpf_assist_sampling_allocator)
	//}
	//if err = readVM(vm, &ebpf_assist_trap, samplerAddr(ebpf_assist_trap_ptr)); err != nil {
	//	return fmt.Errorf("read ebpf_assist_trap_ptr %w", err)
	//}
	//if !pointsTo(uint64(ebpf_assist_trap), pi.PyMemSampler) {
	//	return fmt.Errorf("ebpf_assist_trap points to unknown memory %x %+v", ebpf_assist_trap, pi.PyMemSampler)
	//}
	//var offsetAssistTrap = uint64(ebpf_assist_trap) - pi.PyMemSampler[0].StartAddr
	//var addrAssistDelegateAllocator = samplerAddr(ebpf_assist_delegate_allocator)
	//var addrAssistSamplingAllocator = samplerAddr(ebpf_assist_sampling_allocator)
	//
	//s.logger.Log("op", "pymem sampling", "offsetAssistTrap", fmt.Sprintf("0x%x", offsetAssistTrap))
	//s.logger.Log("op", "pymem sampling", "assist delegate allocator ", fmt.Sprintf("0x%x", addrAssistDelegateAllocator))
	//s.logger.Log("op", "pymem sampling", "trap  ", fmt.Sprintf("0x%x 0x%x", ebpf_assist_trap_ptr, ebpf_assist_trap))
	//s.logger.Log("op", "pymem sampling", "pymemsampler  ", symtab.ProcMaps(pi.PyMemSampler).String())
	//
	//s.logger.Log("op", "pymem sampling", "allocator", allocator.String())
	//if allocatorPointsTo(allocator, getPythonMaps(pi)) {
	//	//TODO  stop app while writing
	//	if err = writeVM(vm, allocator, addrAssistDelegateAllocator); err != nil {
	//		return fmt.Errorf("write current allocator to pysampler %w", err)
	//	}
	//	if err = readVM(vm, allocator, addrAssistSamplingAllocator); err != nil {
	//		return fmt.Errorf("read current allocator from pysampler %w", err)
	//	}
	//	if err = writeVM(vm, allocator, addrPyObjectALlocator); err != nil {
	//		return fmt.Errorf("write current allocator to pysampler %w", err)
	//	}
	//} else if allocatorPointsTo(allocator, pi.PyMemSampler) {
	//	// profiler restart
	//} else {
	//	return fmt.Errorf("allocator points to unknown memory %s expected %s", allocator.String(),
	//		symtab.ProcMaps(pi.PyMemSampler).String())
	//}
	//var trapInstructions uint64
	//if err = readVM(vm, &trapInstructions, ebpf_assist_trap); err != nil {
	//	return fmt.Errorf("read trap instruction %w", err)
	//}
	////if (trapInstructions & 0xff) != 0xc3 { // sanity check
	////	return fmt.Errorf("wrong trap instruction %x", trapInstructions)
	////}
	//exe, err := link.OpenExecutable(pi.PyMemSampler[0].Pathname)
	//if err != nil {
	//	return fmt.Errorf("opening %q executable file: %w", pi.PyMemSampler[0].Pathname, err)
	//}
	//if s.memProg == nil {
	//	return fmt.Errorf("no mem prog")
	//}
	//up, err := exe.Uprobe("", s.memProg, &link.UprobeOptions{
	//	Address: offsetAssistTrap,
	//	PID:     data.PID,
	//})
	//if err != nil {
	//	return fmt.Errorf("setting uprobe: %w", err)
	//}
	//process.memSamplingLink = up
	//return nil
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

func getDlopenPtr(data *ProcData) uint64 {
	glibc := data.ProcInfo.Glibc
	if glibc == nil {
		return 0
	}
	ef, err := elf.Open(fmt.Sprintf("/proc/%d/root%s", data.PID, glibc[0].Pathname))
	if err != nil {
		return 0
	}
	symbols, err := ef.DynamicSymbols()
	if err != nil {
		return 0
	}
	for _, s := range symbols {
		switch s.Name {
		case "dlopen":
			return glibc[0].StartAddr + s.Value
		}
	}
	return 0
}

func getPyObjectAllocatorAddress(m *mem, data *ProcData) int64 {
	fmt.Printf("offsets %+v\n", data.PySymbols)
	if data.PySymbols.PyObject_Free == 0 {
		return 0
	}

	code := make([]byte, 0x80)
	addr := int64(data.PySymbols.BaseAddress + data.PySymbols.PyMem_GetAllocator)
	if _, err := m.ReadAt(code, addr); err != nil {
		return 0
	}
	//0x00000000005f8bc4 <+36>:	cmp    $0x2,%edi
	//0x00000000005f8bc7 <+39>:	jne    0x5f8bd9 <PyMem_GetAllocator+57>
	//0x00000000005f8bc9 <+41>:	mov    $0x94df40,%esi
	//fmt.Printf("code %x = %s\n", addr, hex.EncodeToString(code))
	var instructions []x86asm.Inst
	var found []x86asm.Inst
	for len(code) > 0 {
		if len(code) >= 4 {
			if binary.LittleEndian.Uint32(code) == 0xfa1e0ff3 {
				//fmt.Println("endbr64")
				code = code[4:]
				continue
			}
		}
		inst, err := x86asm.Decode(code, 64)
		if err != nil {
			fmt.Printf(" %s %s", hex.EncodeToString(code), err.Error())
			break
		}

		//it := code[:inst.Len]
		//fmt.Printf("it %10s = %s\n", hex.EncodeToString(it), inst.String())
		code = code[inst.Len:]
		instructions = append(instructions, inst)
	}

	for i, instruction := range instructions {
		/*it     83ff02 = CMP EDI, 0x2
		  it       7510 = JNE .+16
		  it be40df9400 = MOV ESI, 0x94df40*/
		if instruction.String() == "CMP EDI, 0x2" {
			found = instructions[i:]
			break
		}
	}
	if len(found) < 3 {
		return 0
	}
	found = found[:3]
	for _, it := range found {
		fmt.Printf("found %s\n", it.String())
	}
	it := found[1].String()
	if !strings.HasPrefix(it, "JNE") {
		fmt.Printf(">>>JNE >>%s<<", it)
		return 0
	}
	if !strings.HasPrefix(found[2].String(), "MOV ESI") {
		fmt.Println(">>>ESI")
		return 0
	}
	re := regexp.MustCompile("MOV ESI, 0x([0-9a-f]+)")
	submatch := re.FindStringSubmatch(found[2].String())
	if submatch == nil {
		fmt.Println(">>>NOMATCH")
		return 0
	}
	addr, err := strconv.ParseInt(submatch[1], 16, 64)
	if err != nil {
		fmt.Println(">>>" + err.Error())
		return 0
	}
	fmt.Printf("found 0x%x\n", addr)

	return int64(addr)

}

//go:embed pymemsampler/injector
var injectorTemplate []byte

func getInjector(data *ProcData, freePtr, frePtrPtr int64, soPath string) ([]byte, error) {
	res := make([]byte, len(injectorTemplate))
	copy(res, injectorTemplate)

	ptrs := res[len(res)-24:]
	dlopen := getDlopenPtr(data)
	if dlopen == 0 {
		return nil, fmt.Errorf("dlopen not found")
	}
	//dlopen_ptr: dq 0
	//free_ptr: dq 0
	//free_ptr_ptr: dq 0
	binary.LittleEndian.PutUint64(ptrs, dlopen)
	binary.LittleEndian.PutUint64(ptrs[8:], uint64(freePtr))
	binary.LittleEndian.PutUint64(ptrs[16:], uint64(frePtrPtr))
	res = append(res, []byte(soPath)...)
	return res, nil
}

func writeInjector(m *mem, data *ProcData, injector []byte) (int64, error) {
	maps := getPythonMaps(data.ProcInfo)
	rx := symtab.FindLastRXMap(maps)
	if rx == nil {
		return 0, fmt.Errorf("could not find a place to put injector %s", symtab.ProcMaps(maps))
	}
	at := int64(rx.EndAddr - uint64(len(injector)))
	at /= 0x10
	at *= 0x10
	at -= 0x10
	actual := make([]byte, len(injector))
	expected := make([]byte, len(injector))

	if _, err := m.ReadAt(actual, int64(at)); err != nil {
		return 0, err
	}
	if !bytes.Equal(expected, actual) {
		return 0, fmt.Errorf("unexpected code at %x expected %s got %s", at, hex.EncodeToString(expected), hex.EncodeToString(actual))
	}
	if _, err := m.WriteAt(injector, int64(at)); err != nil {
		return 0, fmt.Errorf("writing injector at %x %w", at, err)
	}

	return at, nil
}
