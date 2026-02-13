package querydiagnostics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	memindex "github.com/grafana/pyroscope/pkg/segmentwriter/memdb/index"
)

// BlockAnonymizer anonymizes blocks by replacing sensitive strings in the symbol
// database with deterministic hashes. Profiles (parquet) and TSDB index sections
// are raw-byte-copied — they don't contain symbol strings. Only the symbols
// section is rewritten: read partitions, anonymize strings, write new symdb.
type BlockAnonymizer struct {
	bucket objstore.Bucket

	mu        sync.Mutex
	stringMap map[string]string
}

func NewBlockAnonymizer(bucket objstore.Bucket) *BlockAnonymizer {
	return &BlockAnonymizer{
		bucket:    bucket,
		stringMap: make(map[string]string),
	}
}

// AnonymizeBlock reads a block, anonymizes its symbol data, and returns the
// anonymized block as bytes. Stacktrace IDs in the copied parquet rows remain
// valid because we preserve all indices: strings, functions, mappings, locations
// are appended with sequential IDs matching their original positions.
func (a *BlockAnonymizer) AnonymizeBlock(ctx context.Context, meta *metastorev1.BlockMeta) ([]byte, error) {
	obj := block.NewObject(a.bucket, meta)
	defer obj.Close()
	if err := obj.Open(ctx); err != nil {
		return nil, fmt.Errorf("opening block: %w", err)
	}

	fullMeta, err := obj.ReadMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading full metadata: %w", err)
	}
	obj.SetMetadata(fullMeta)

	var buf bytes.Buffer
	w := &writerWithOffset{w: &buf}

	// Collect dataset names referenced by the query plan so we can skip
	// datasets that the query didn't touch.
	planDatasets := make(map[string]struct{})
	for _, ds := range meta.Datasets {
		if int(ds.Name) < len(meta.StringTable) {
			if name := meta.StringTable[ds.Name]; name != "" {
				planDatasets[name] = struct{}{}
			}
		}
	}

	oldStrings := fullMeta.StringTable

	newMeta := &metastorev1.BlockMeta{
		FormatVersion:   fullMeta.FormatVersion,
		Id:              fullMeta.Id,
		Shard:           fullMeta.Shard,
		CompactionLevel: fullMeta.CompactionLevel,
		MinTime:         fullMeta.MinTime,
		MaxTime:         fullMeta.MaxTime,
	}
	stringTable := metadata.NewStringTable()
	newMeta.Tenant = stringTable.Put(metadata.Tenant(fullMeta))
	if int(fullMeta.CreatedBy) < len(oldStrings) {
		newMeta.CreatedBy = stringTable.Put(oldStrings[fullMeta.CreatedBy])
	}

	// Track old -> new dataset position mapping so we can rebuild the
	// dataset index with correct references after filtering.
	positionMap := make(map[uint32]uint32)
	var datasetIndexMeta *metastorev1.Dataset

	for i, dsMeta := range fullMeta.Datasets {
		if block.DatasetFormat(dsMeta.Format) == block.DatasetFormat1 {
			datasetIndexMeta = dsMeta
			continue
		}
		if dsMeta.Name == 0 {
			continue
		}
		if len(planDatasets) > 0 {
			dsName := ""
			if int(dsMeta.Name) < len(oldStrings) {
				dsName = oldStrings[dsMeta.Name]
			}
			if _, ok := planDatasets[dsName]; !ok {
				continue
			}
		}

		ds := block.NewDataset(dsMeta, obj)
		if err := ds.Open(ctx, block.SectionSymbols); err != nil {
			return nil, fmt.Errorf("opening dataset %s: %w", ds.Name(), err)
		}

		newPosition := uint32(len(newMeta.Datasets))
		positionMap[uint32(i)] = newPosition

		anonDS, err := a.anonymizeDataset(ctx, w, ds, obj, oldStrings, stringTable)
		ds.Close()
		if err != nil {
			return nil, fmt.Errorf("anonymizing dataset %s: %w", ds.Name(), err)
		}
		newMeta.Datasets = append(newMeta.Datasets, anonDS)
	}

	// Rebuild the dataset index with remapped positions, appending it
	// after all Format0 datasets (matching the normal compaction layout).
	if datasetIndexMeta != nil {
		anonDS, err := a.rebuildDatasetIndex(ctx, w, datasetIndexMeta, obj, positionMap, oldStrings, stringTable)
		if err != nil {
			return nil, fmt.Errorf("rebuilding dataset index: %w", err)
		}
		newMeta.Datasets = append(newMeta.Datasets, anonDS)
	}

	newMeta.StringTable = stringTable.Strings
	newMeta.MetadataOffset = w.offset
	if err := metadata.Encode(&buf, newMeta); err != nil {
		return nil, fmt.Errorf("encoding metadata: %w", err)
	}
	newMeta.Size = uint64(buf.Len())

	return buf.Bytes(), nil
}

func (a *BlockAnonymizer) anonymizeDataset(
	ctx context.Context,
	w *writerWithOffset,
	ds *block.Dataset,
	obj *block.Object,
	oldStrings []string,
	stringTable *metadata.StringTable,
) (*metastorev1.Dataset, error) {
	dsMeta := ds.Metadata()
	startOffset := w.offset

	profilesOff := dsMeta.TableOfContents[0]
	tsdbOff := dsMeta.TableOfContents[1]
	symbolsOff := dsMeta.TableOfContents[2]
	datasetEnd := profilesOff + dsMeta.Size

	profilesSize := tsdbOff - profilesOff
	tsdbSize := symbolsOff - tsdbOff
	_ = datasetEnd - symbolsOff // symbolsSize

	blockPath := block.ObjectPath(obj.Metadata())
	rawData, err := a.readRange(ctx, blockPath, int64(profilesOff), int64(profilesSize+tsdbSize))
	if err != nil {
		return nil, fmt.Errorf("reading profiles+tsdb sections: %w", err)
	}

	newProfilesOff := w.offset
	if _, err := w.Write(rawData[:profilesSize]); err != nil {
		return nil, fmt.Errorf("writing profiles: %w", err)
	}

	newTsdbOff := w.offset
	if _, err := w.Write(rawData[profilesSize:]); err != nil {
		return nil, fmt.Errorf("writing tsdb: %w", err)
	}

	newSymbolsOff := w.offset
	if err := a.anonymizeAndWriteSymbols(ctx, w, ds); err != nil {
		return nil, fmt.Errorf("anonymizing symbols: %w", err)
	}

	labels := reindexLabels(dsMeta.Labels, oldStrings, stringTable)

	return &metastorev1.Dataset{
		Format:          dsMeta.Format,
		Tenant:          stringTable.Put(ds.TenantID()),
		Name:            stringTable.Put(ds.Name()),
		MinTime:         dsMeta.MinTime,
		MaxTime:         dsMeta.MaxTime,
		TableOfContents: []uint64{newProfilesOff, newTsdbOff, newSymbolsOff},
		Size:            w.offset - startOffset,
		Labels:          labels,
	}, nil
}

// rebuildDatasetIndex reads the original dataset index, remaps the dataset
// position references to match the new metadata layout, and writes a new
// index. This is necessary because anonymization may drop datasets,
// changing their positions in the metadata.
func (a *BlockAnonymizer) rebuildDatasetIndex(
	ctx context.Context,
	w *writerWithOffset,
	dsMeta *metastorev1.Dataset,
	obj *block.Object,
	positionMap map[uint32]uint32,
	oldStrings []string,
	stringTable *metadata.StringTable,
) (*metastorev1.Dataset, error) {
	if len(dsMeta.TableOfContents) == 0 {
		return nil, fmt.Errorf("dataset index has no table of contents")
	}

	sectionOff := dsMeta.TableOfContents[0]
	blockPath := block.ObjectPath(obj.Metadata())
	rawData, err := a.readRange(ctx, blockPath, int64(sectionOff), int64(dsMeta.Size))
	if err != nil {
		return nil, fmt.Errorf("reading dataset index: %w", err)
	}

	reader, err := index.NewReader(index.RealByteSlice(rawData))
	if err != nil {
		return nil, fmt.Errorf("opening dataset index reader: %w", err)
	}
	defer reader.Close()

	// Iterate all series and collect the ones whose dataset was kept.
	type seriesEntry struct {
		labels      phlaremodel.Labels
		fingerprint model.Fingerprint
		newIndex    uint32
	}

	k, v := index.AllPostingsKey()
	postings, err := reader.Postings(k, nil, v)
	if err != nil {
		return nil, fmt.Errorf("reading dataset index postings: %w", err)
	}

	symbols := make(map[string]struct{})
	var entries []seriesEntry
	var lbls phlaremodel.Labels
	var chunks []index.ChunkMeta

	for postings.Next() {
		fp, err := reader.Series(postings.At(), &lbls, &chunks)
		if err != nil {
			return nil, fmt.Errorf("reading dataset index series: %w", err)
		}
		if len(chunks) == 0 {
			continue
		}
		oldPos := chunks[0].SeriesIndex
		newPos, ok := positionMap[oldPos]
		if !ok {
			continue
		}
		cloned := make(phlaremodel.Labels, len(lbls))
		copy(cloned, lbls)
		for _, l := range cloned {
			symbols[l.Name] = struct{}{}
			symbols[l.Value] = struct{}{}
		}
		entries = append(entries, seriesEntry{
			labels:      cloned,
			fingerprint: model.Fingerprint(fp),
			newIndex:    newPos,
		})
	}
	if err := postings.Err(); err != nil {
		return nil, fmt.Errorf("iterating dataset index postings: %w", err)
	}

	// Write the new dataset index.
	iw, err := memindex.NewWriter(ctx, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("creating dataset index writer: %w", err)
	}

	sortedSymbols := make([]string, 0, len(symbols))
	for s := range symbols {
		sortedSymbols = append(sortedSymbols, s)
	}
	sort.Strings(sortedSymbols)
	for _, s := range sortedSymbols {
		if err := iw.AddSymbol(s); err != nil {
			return nil, fmt.Errorf("adding symbol: %w", err)
		}
	}

	for i, e := range entries {
		chunk := index.ChunkMeta{SeriesIndex: e.newIndex}
		if err := iw.AddSeries(storage.SeriesRef(i), e.labels, e.fingerprint, chunk); err != nil {
			return nil, fmt.Errorf("adding series: %w", err)
		}
	}

	if err := iw.Close(); err != nil {
		return nil, fmt.Errorf("closing dataset index writer: %w", err)
	}
	newIndexData := iw.ReleaseIndex()

	startOffset := w.offset
	newSectionOff := w.offset
	if _, err := w.Write(newIndexData); err != nil {
		return nil, fmt.Errorf("writing dataset index: %w", err)
	}

	labels := reindexLabels(dsMeta.Labels, oldStrings, stringTable)

	var tenantRef int32
	if int(dsMeta.Tenant) < len(oldStrings) {
		tenantRef = stringTable.Put(oldStrings[dsMeta.Tenant])
	}

	return &metastorev1.Dataset{
		Format:          dsMeta.Format,
		Tenant:          tenantRef,
		Name:            0,
		MinTime:         dsMeta.MinTime,
		MaxTime:         dsMeta.MaxTime,
		TableOfContents: []uint64{newSectionOff},
		Size:            w.offset - startOffset,
		Labels:          labels,
	}, nil
}

// reindexLabels translates dataset label references from the old string table
// to the new one. The label format is length-prefixed groups:
// [count, k, v, k, v, ..., count, k, v, ...] where count positions are literal
// integers (not string refs) and must be preserved as-is.
func reindexLabels(oldLabels []int32, oldStrings []string, stringTable *metadata.StringTable) []int32 {
	labels := make([]int32, len(oldLabels))
	copy(labels, oldLabels)
	var skip int
	for i, v := range labels {
		if i == skip {
			skip += int(v)*2 + 1
			continue
		}
		if int(v) < len(oldStrings) {
			labels[i] = stringTable.Put(oldStrings[v])
		}
	}
	return labels
}

func (a *BlockAnonymizer) readRange(ctx context.Context, path string, offset, size int64) ([]byte, error) {
	rc, err := a.bucket.GetRange(ctx, path, offset, size)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (a *BlockAnonymizer) anonymizeAndWriteSymbols(
	ctx context.Context,
	w io.Writer,
	ds *block.Dataset,
) error {
	symbols := ds.Symbols()
	if symbols == nil {
		return nil
	}
	reader, ok := symbols.(*symdb.Reader)
	if !ok {
		return fmt.Errorf("unsupported symbols reader type for string rewriting")
	}
	return reader.RewriteStrings(ctx, a.anonymizeString, w)
}

func (a *BlockAnonymizer) anonymizeString(s string) string {
	if s == "" {
		return ""
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if anon, ok := a.stringMap[s]; ok {
		return anon
	}

	hash := sha256.Sum256([]byte(s))
	anon := hex.EncodeToString(hash[:8])
	a.stringMap[s] = anon
	return anon
}

// extractL3Blocks walks the query plan DAG and collects blocks with compaction_level == 3,
// deduplicating by block ID.
func extractL3Blocks(plan *queryv1.QueryPlan) []*metastorev1.BlockMeta {
	if plan == nil || plan.Root == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var blocks []*metastorev1.BlockMeta
	collectL3Blocks(plan.Root, seen, &blocks)
	return blocks
}

func collectL3Blocks(node *queryv1.QueryNode, seen map[string]struct{}, blocks *[]*metastorev1.BlockMeta) {
	if node == nil {
		return
	}
	if node.Type == queryv1.QueryNode_READ {
		for _, b := range node.Blocks {
			if b.CompactionLevel == 3 {
				if _, ok := seen[b.Id]; !ok {
					seen[b.Id] = struct{}{}
					*blocks = append(*blocks, b)
				}
			}
		}
	}
	for _, child := range node.Children {
		collectL3Blocks(child, seen, blocks)
	}
}

type writerWithOffset struct {
	w      io.Writer
	offset uint64
}

func (w *writerWithOffset) Write(p []byte) (n int, err error) {
	n, err = w.w.Write(p)
	w.offset += uint64(n)
	return n, err
}

