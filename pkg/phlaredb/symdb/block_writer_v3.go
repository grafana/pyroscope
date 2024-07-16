package symdb

import (
	"fmt"
	"hash/crc32"
	"io"
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

	stringsEncoder   *symbolsEncoder[string]
	mappingsEncoder  *symbolsEncoder[v1.InMemoryMapping]
	functionsEncoder *symbolsEncoder[v1.InMemoryFunction]
	locationsEncoder *symbolsEncoder[v1.InMemoryLocation]
}

func newWriterV3(c *Config) *writerV3 {
	return &writerV3{
		config: c,
		index: IndexFile{
			Header: IndexHeader{
				Magic:   symdbMagic,
				Version: FormatV3,
			},
		},
		footer: Footer{
			Magic:   symdbMagic,
			Version: FormatV3,
		},

		stringsEncoder:   newStringsEncoder(),
		mappingsEncoder:  newMappingsEncoder(),
		functionsEncoder: newFunctionsEncoder(),
		locationsEncoder: newLocationsEncoder(),
	}
}

func (w *writerV3) writePartitions(partitions []*PartitionWriter) (err error) {
	if err = w.config.Fs.MkdirAll(w.config.Dir, 0o755); err != nil {
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
		if err = writePartitionV3(w, p); err != nil {
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
	if f, err = newFileWriter(w.config.Fs, path); err != nil {
		return nil, fmt.Errorf("failed to create %q: %w", path, err)
	}
	return f, err
}

func writePartitionV3(w *writerV3, p *PartitionWriter) (err error) {
	if p.header.V3.Strings, err = writeSymbolsBlock(w.dataFile, p.strings.slice, w.stringsEncoder); err != nil {
		return err
	}
	if p.header.V3.Mappings, err = writeSymbolsBlock(w.dataFile, p.mappings.slice, w.mappingsEncoder); err != nil {
		return err
	}
	if p.header.V3.Functions, err = writeSymbolsBlock(w.dataFile, p.functions.slice, w.functionsEncoder); err != nil {
		return err
	}
	if p.header.V3.Locations, err = writeSymbolsBlock(w.dataFile, p.locations.slice, w.locationsEncoder); err != nil {
		return err
	}
	for ci, c := range p.stacktraces.chunks {
		stacks := c.stacks
		if stacks == 0 {
			stacks = uint32(len(p.stacktraces.hashToIdx))
		}
		h := StacktraceBlockHeader{
			Offset:             w.dataFile.w.offset,
			Partition:          p.header.Partition,
			BlockIndex:         uint16(ci),
			Encoding:           StacktraceEncodingGroupVarint,
			Stacktraces:        stacks,
			StacktraceNodes:    c.tree.len(),
			StacktraceMaxNodes: c.partition.maxNodesPerChunk,
		}
		crc := crc32.New(castagnoli)
		if h.Size, err = c.WriteTo(io.MultiWriter(crc, w.dataFile)); err != nil {
			return fmt.Errorf("writing stacktrace chunk data: %w", err)
		}
		h.CRC = crc.Sum32()
		p.header.Stacktraces = append(p.header.Stacktraces, h)
	}
	return nil
}

func writeSymbolsBlock[T any](w *fileWriter, s []T, e *symbolsEncoder[T]) (h SymbolsBlockHeader, err error) {
	h.Offset = uint64(w.w.offset)
	crc := crc32.New(castagnoli)
	mw := io.MultiWriter(crc, w.w)
	if err = e.encode(mw, s); err != nil {
		return h, err
	}
	h.Size = uint32(w.w.offset) - uint32(h.Offset)
	h.CRC = crc.Sum32()
	h.Length = uint32(len(s))
	h.BlockSize = uint32(e.blockSize)
	h.BlockHeaderSize = uint16(e.blockEncoder.headerSize())
	h.Format = e.blockEncoder.format()
	return h, nil
}
