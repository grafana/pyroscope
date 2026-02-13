package symdb

import (
	"context"
	"fmt"
	"hash/crc32"
	"io"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// RewriteStrings rewrites the symdb, applying the transform function to every
// string while raw-copying all other sections (mappings, functions, locations,
// stacktraces) byte-for-byte. This is much faster than decoding and
// re-encoding the entire symdb because stacktrace tree construction (the
// dominant cost) is completely avoided.
//
// The caller must ensure that the transform preserves string count (1:1
// mapping); it must not add or remove strings. The indices used by functions,
// mappings, and locations remain valid because they reference strings by
// position and the positions do not change.
func (r *Reader) RewriteStrings(ctx context.Context, transform func(string) string, dst io.Writer) error {
	w := withWriterOffset(dst)
	index := newIndexFileV3()
	footer := newFooterV3()
	enc := newStringsEncoder()

	for _, p := range r.partitions {
		header, err := rewritePartitionStrings(ctx, w, r, p, enc, transform)
		if err != nil {
			return fmt.Errorf("rewriting partition: %w", err)
		}
		index.PartitionHeaders = append(index.PartitionHeaders, header)
	}

	footer.IndexOffset = uint64(w.offset)
	if _, err := index.WriteTo(w); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}
	if _, err := w.Write(footer.MarshalBinary()); err != nil {
		return fmt.Errorf("writing footer: %w", err)
	}
	return nil
}

func rewritePartitionStrings(
	ctx context.Context,
	w *writerOffset,
	r *Reader,
	p *partition,
	enc *symbolsEncoder[string],
	transform func(string) string,
) (*PartitionHeader, error) {
	st, ok := p.strings.(*rawTable[string])
	if !ok {
		return nil, fmt.Errorf("unsupported partition format for string rewriting")
	}
	mt := p.mappings.(*rawTable[schemav1.InMemoryMapping])
	ft := p.functions.(*rawTable[schemav1.InMemoryFunction])
	lt := p.locations.(*rawTable[schemav1.InMemoryLocation])

	// Fetch and decode only the strings.
	if err := st.fetch(ctx); err != nil {
		return nil, fmt.Errorf("fetching strings: %w", err)
	}
	defer st.release()

	// Apply the transform.
	strings := st.slice()
	transformed := make([]string, len(strings))
	for i, s := range strings {
		transformed[i] = transform(s)
	}

	// Write the partition data. The on-disk order is:
	//   strings, mappings, functions, locations, stacktraces.

	header := &PartitionHeader{
		V3: new(PartitionHeaderV3),
	}

	// Strings: encode from the transformed slice.
	var err error
	if header.V3.Strings, err = writeSymbolsBlock(w, transformed, enc); err != nil {
		return nil, fmt.Errorf("writing strings: %w", err)
	}

	// Mappings, functions, locations: raw-copy from storage.
	if header.V3.Mappings, err = rawCopySymbolsBlock(ctx, w, r, mt.header); err != nil {
		return nil, fmt.Errorf("copying mappings: %w", err)
	}
	if header.V3.Functions, err = rawCopySymbolsBlock(ctx, w, r, ft.header); err != nil {
		return nil, fmt.Errorf("copying functions: %w", err)
	}
	if header.V3.Locations, err = rawCopySymbolsBlock(ctx, w, r, lt.header); err != nil {
		return nil, fmt.Errorf("copying locations: %w", err)
	}

	// Stacktraces: raw-copy each block.
	for _, sb := range p.stacktraces {
		sh, err := rawCopyStacktraceBlock(ctx, w, r, sb.header)
		if err != nil {
			return nil, fmt.Errorf("copying stacktraces: %w", err)
		}
		header.Stacktraces = append(header.Stacktraces, sh)
	}

	// Partition ID must match the original.
	if len(p.stacktraces) > 0 {
		header.Partition = p.stacktraces[0].header.Partition
	}

	return header, nil
}

// rawCopySymbolsBlock copies a symbols block (mappings, functions, or
// locations) verbatim from the source reader to the destination writer.
// It updates the Offset field in the returned header to reflect the new
// position; all other fields (Size, CRC, Length, etc.) are unchanged.
func rawCopySymbolsBlock(ctx context.Context, w *writerOffset, r *Reader, src SymbolsBlockHeader) (SymbolsBlockHeader, error) {
	dst := src
	dst.Offset = uint64(w.offset)
	rc, err := r.bucket.GetRange(ctx, r.file.RelPath, int64(src.Offset), int64(src.Size))
	if err != nil {
		return dst, err
	}
	defer rc.Close()
	if _, err := io.Copy(w, rc); err != nil {
		return dst, err
	}
	return dst, nil
}

// rawCopyStacktraceBlock copies a stacktrace block verbatim from the source
// reader to the destination writer, updating the Offset field.
func rawCopyStacktraceBlock(ctx context.Context, w *writerOffset, r *Reader, src StacktraceBlockHeader) (StacktraceBlockHeader, error) {
	dst := src
	dst.Offset = w.offset

	rc, err := r.bucket.GetRange(ctx, r.file.RelPath, src.Offset, src.Size)
	if err != nil {
		return dst, err
	}
	defer rc.Close()

	crc := crc32.New(castagnoli)
	n, err := io.Copy(io.MultiWriter(w, crc), rc)
	if err != nil {
		return dst, err
	}
	dst.Size = n
	dst.CRC = crc.Sum32()
	return dst, nil
}
