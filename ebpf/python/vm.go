package python

import (
	"fmt"
	"os"
	"unsafe"
)

type mem struct {
	pid int
	f   *os.File
}

func (m *mem) u64() (uint64, error) {
	var res uint64
	err := readVM(m, &res, 0)
	return res, err
}

func (m *mem) Close() error {
	return m.f.Close()
}

func newVM(pid int) (*mem, error) {
	f, err := os.OpenFile(fmt.Sprintf("/proc/%d/mem", pid), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &mem{pid: pid, f: f}, nil
}

func readVM[T any](v *mem, p *T, at int64) error {
	slice := unsafe.Slice((*byte)(unsafe.Pointer(p)), unsafe.Sizeof(*p))
	_, err := v.ReadAt(slice, at)
	return err
}

func (v *mem) ReadAt(b []byte, off int64) (n int, err error) {
	return v.f.ReadAt(b, off)
}

func writeVM[T any](v *mem, p *T, at int64) error {
	slice := unsafe.Slice((*byte)(unsafe.Pointer(p)), unsafe.Sizeof(*p))
	_, err := v.WriteAt(slice, at)
	return err
}

func (v *mem) WriteAt(b []byte, off int64) (n int, err error) {
	return v.f.WriteAt(b, off)
}
