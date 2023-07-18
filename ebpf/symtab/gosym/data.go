package gosym

import (
	"io"
	"os"

	bufra "github.com/avvmoto/buf-readerat"
)

type PCLNData interface {
	ReadAt(data []byte, offset int) error
}

type MemPCLNData struct {
	Data []byte
}

func (m MemPCLNData) ReadAt(data []byte, offset int) error {
	copy(data, m.Data[offset:])
	return nil
}

type FilePCLNData struct {
	file   io.ReaderAt
	offset int
}

func NewFilePCLNData(f *os.File, offset int) *FilePCLNData {
	return &FilePCLNData{
		file:   bufra.NewBufReaderAt(f, 4*0x1000),
		offset: offset,
	}
}

func (f *FilePCLNData) ReadAt(data []byte, offset int) error {
	n, err := f.file.ReadAt(data, int64(offset+f.offset))
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.EOF
	}
	return nil
}
