package symdb

import (
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type writerV3 struct {
	config *Config

	index    IndexFile
	dataFile *fileWriter
	files    []block.File
	footer   Footer

	encodersV3
}

type encodersV3 struct {
	stringsEncoder   *symbolsEncoder[string]
	mappingsEncoder  *symbolsEncoder[v1.InMemoryMapping]
	functionsEncoder *symbolsEncoder[v1.InMemoryFunction]
	locationsEncoder *symbolsEncoder[v1.InMemoryLocation]
}

func newWriterV3(c *Config) *writerV3 {
	return &writerV3{
		config:     c,
		index:      newIndexFileV3(),
		footer:     newFooterV3(),
		encodersV3: newEncodersV3(),
	}
}

func (w *writerV3) writePartitions(partitions []*PartitionWriter) (err error) {
	if err = os.MkdirAll(w.config.Dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", w.config.Dir, err)
	}
	if w.dataFile, err = w.newFile(DefaultFileName); err != nil {
		return err
	}
	defer func() {
		err = w.dataFile.Close()
		w.files = []block.File{w.dataFile.meta()}
	}()
	for _, p := range partitions {
		if err = writePartitionV3(w.dataFile.w, &w.encodersV3, p); err != nil {
			return fmt.Errorf("failed to write partition: %w", err)
		}
		w.index.PartitionHeaders = append(w.index.PartitionHeaders, &p.header)
	}
	w.footer.IndexOffset = uint64(w.dataFile.w.offset)
	if _, err = w.index.WriteTo(w.dataFile); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}
	if _, err = w.dataFile.Write(w.footer.MarshalBinary()); err != nil {
		return fmt.Errorf("failed to write footer: %w", err)
	}
	return nil
}

func (w *writerV3) meta() []block.File { return w.files }

func (w *writerV3) newFile(path string) (f *fileWriter, err error) {
	path = filepath.Join(w.config.Dir, path)
	if f, err = newFileWriter(path); err != nil {
		return nil, fmt.Errorf("failed to create %q: %w", path, err)
	}
	return f, err
}

func writePartitionV3(w *writerOffset, e *encodersV3, p *PartitionWriter) (err error) {
	if p.header.V3.Strings, err = writeSymbolsBlock(w, p.strings.slice, e.stringsEncoder); err != nil {
		return err
	}
	if p.header.V3.Mappings, err = writeSymbolsBlock(w, p.mappings.slice, e.mappingsEncoder); err != nil {
		return err
	}
	if p.header.V3.Functions, err = writeSymbolsBlock(w, p.functions.slice, e.functionsEncoder); err != nil {
		return err
	}
	if p.header.V3.Locations, err = writeSymbolsBlock(w, p.locations.slice, e.locationsEncoder); err != nil {
		return err
	}

	h := StacktraceBlockHeader{
		Offset:             w.offset,
		Partition:          p.header.Partition,
		Encoding:           StacktraceEncodingGroupVarint,
		Stacktraces:        uint32(len(p.stacktraces.hashToIdx)),
		StacktraceNodes:    p.stacktraces.tree.len(),
		StacktraceMaxNodes: math.MaxUint32,
	}
	crc := crc32.New(castagnoli)
	if h.Size, err = p.stacktraces.WriteTo(io.MultiWriter(crc, w)); err != nil {
		return fmt.Errorf("writing stacktrace chunk data: %w", err)
	}
	h.CRC = crc.Sum32()
	p.header.Stacktraces = append(p.header.Stacktraces, h)

	return nil
}

func writeSymbolsBlock[T any](w *writerOffset, s []T, e *symbolsEncoder[T]) (h SymbolsBlockHeader, err error) {
	h.Offset = uint64(w.offset)
	crc := crc32.New(castagnoli)
	mw := io.MultiWriter(crc, w)
	if err = e.encode(mw, s); err != nil {
		return h, err
	}
	h.Size = uint32(w.offset) - uint32(h.Offset)
	h.CRC = crc.Sum32()
	h.Length = uint32(len(s))
	h.BlockSize = uint32(e.blockSize)
	h.BlockHeaderSize = uint16(e.blockEncoder.headerSize())
	h.Format = e.blockEncoder.format()
	return h, nil
}

func WritePartition(p *PartitionWriter, dst io.Writer) error {
	index := newIndexFileV3()
	footer := newFooterV3()
	encoders := newEncodersV3()
	w := withWriterOffset(dst)

	if err := writePartitionV3(w, &encoders, p); err != nil {
		return fmt.Errorf("failed to write partition: %w", err)
	}
	index.PartitionHeaders = append(index.PartitionHeaders, &p.header)
	footer.IndexOffset = uint64(w.offset)
	if _, err := index.WriteTo(w); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}
	if _, err := w.Write(footer.MarshalBinary()); err != nil {
		return fmt.Errorf("failed to write footer: %w", err)
	}
	return nil
}

func newEncodersV3() encodersV3 {
	return encodersV3{
		stringsEncoder:   newStringsEncoder(),
		mappingsEncoder:  newMappingsEncoder(),
		functionsEncoder: newFunctionsEncoder(),
		locationsEncoder: newLocationsEncoder(),
	}
}

func newFooterV3() Footer {
	return Footer{
		Magic:   symdbMagic,
		Version: FormatV3,
	}
}

func newIndexFileV3() IndexFile {
	return IndexFile{
		Header: IndexHeader{
			Magic:   symdbMagic,
			Version: FormatV3,
		},
	}
}
