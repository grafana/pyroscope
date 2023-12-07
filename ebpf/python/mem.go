package python

import (
	"debug/elf"
	"fmt"
)

//typedef struct {
//void *ctx;
//
//void *(*malloc)(void *ctx, size_t size);
//
//void *(*calloc)(void *ctx, size_t nelem, size_t elsize);
//
//void *(*realloc)(void *ctx, void *ptr, size_t new_size);
//
//void (*free)(void *ctx, void *ptr);
//} PyMemAllocatorEx;
//void ebpf_assist_trap(size_t size) {

func (s *Perf) InitMemSampling(data *ProcData) error {
	pi := data.ProcInfo
	if pi.PyMemSampler == nil {
		return nil
	}
	ef, err := elf.Open(pi.PyMemSampler[0].Pathname)
	if err != nil {
		return err
	}
	symbols, err := ef.DynamicSymbols()
	if err != nil {
		return err
	}
	var (
		python_allocator_impl          uintptr
		ebpf_assist_trap               uintptr
		ebpf_assist_sampling_allocator uintptr
	)

	for _, s := range symbols {
		switch s.Name {
		case "ebpf_assist_trap":
			ebpf_assist_trap = uintptr(s.Value)
		case "python_allocator_impl":
			python_allocator_impl = uintptr(s.Value)
		case "ebpf_assist_sampling_allocator":
			ebpf_assist_sampling_allocator = uintptr(s.Value)
		}
	}
	if python_allocator_impl == 0 || ebpf_assist_trap == 0 || ebpf_assist_sampling_allocator == 0 {
		return fmt.Errorf("wrong lib %x %x %x", python_allocator_impl, ebpf_assist_trap, ebpf_assist_sampling_allocator)
	}

	return nil
}
