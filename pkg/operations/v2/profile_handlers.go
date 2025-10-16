package v2

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

func (h *Handlers) CreateDatasetProfilesHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := parseDatasetRequest(r)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		page := 1
		if pageStr := r.URL.Query().Get("page"); pageStr != "" {
			if _, err := fmt.Sscanf(pageStr, "%d", &page); err != nil || page < 1 {
				page = 1
			}
		}

		pageSize := 100
		if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
			if _, err := fmt.Sscanf(pageSizeStr, "%d", &pageSize); err != nil || pageSize < 1 || pageSize > 500 {
				pageSize = 100
			}
		}

		blockMeta, foundDataset, err := h.getDatasetMetadata(r.Context(), req)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		dataset := h.convertDataset(foundDataset, blockMeta.StringTable)

		profiles, totalCount, err := h.readProfilesFromDataset(r.Context(), blockMeta, foundDataset, page, pageSize)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to read profiles from dataset"))
			return
		}

		totalPages := (totalCount + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}

		err = pageTemplates.datasetProfilesTemplate.Execute(w, datasetProfilesPageContent{
			User:        req.TenantID,
			BlockID:     req.BlockID,
			Shard:       req.Shard,
			BlockTenant: req.BlockTenant,
			Dataset:     &dataset,
			Profiles:    profiles,
			TotalCount:  totalCount,
			Page:        page,
			PageSize:    pageSize,
			TotalPages:  totalPages,
			HasPrevPage: page > 1,
			HasNextPage: page < totalPages,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) readProfilesFromDataset(ctx context.Context, blockMeta *metastorev1.BlockMeta, dataset *metastorev1.Dataset, page, pageSize int) ([]profileInfo, int, error) {
	obj := block.NewObject(h.Bucket, blockMeta)
	if err := obj.Open(ctx); err != nil {
		return nil, 0, fmt.Errorf("failed to open block object: %w", err)
	}
	defer obj.Close()

	ds := block.NewDataset(dataset, obj)
	if err := ds.Open(ctx, block.SectionProfiles, block.SectionTSDB); err != nil {
		return nil, 0, fmt.Errorf("failed to open dataset: %w", err)
	}
	defer ds.Close()

	it, err := block.NewProfileRowIterator(ds)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create profile iterator: %w", err)
	}
	defer it.Close()

	var profiles []profileInfo
	rowNumber := 0
	totalCount := 0
	startRow := (page - 1) * pageSize
	endRow := startRow + pageSize

	for it.Next() {
		if rowNumber >= startRow && rowNumber < endRow {
			entry := it.At()

			var labelsStr strings.Builder
			labelsStr.WriteString("{")
			for i, label := range entry.Labels {
				if i > 0 {
					labelsStr.WriteString(", ")
				}
				labelsStr.WriteString(label.Name)
				labelsStr.WriteString("=")
				labelsStr.WriteString(label.Value)
			}
			labelsStr.WriteString("}")

			var sampleCount int
			var annotationsStr strings.Builder

			entry.Row.ForStacktraceIdsAndValues(func(sids []parquet.Value, vals []parquet.Value) {
				sampleCount = len(sids)
			})

			entry.Row.ForAnnotations(func(keys []parquet.Value, values []parquet.Value) {
				for i, key := range keys {
					if i > 0 {
						annotationsStr.WriteString(", ")
					}
					annotationsStr.WriteString(string(key.ByteArray()))
					annotationsStr.WriteString("=")
					if i < len(values) {
						annotationsStr.WriteString(string(values[i].ByteArray()))
					}
				}
			})

			profiles = append(profiles, profileInfo{
				RowNumber:   rowNumber,
				Timestamp:   time.Unix(0, entry.Timestamp).UTC().Format(time.RFC3339),
				SeriesIndex: entry.Row.SeriesIndex(),
				Fingerprint: uint64(entry.Fingerprint),
				Labels:      labelsStr.String(),
				TotalValue:  uint64(entry.Row.TotalValue()),
				PartitionID: entry.Row.StacktracePartitionID(),
				SampleCount: sampleCount,
				Annotations: annotationsStr.String(),
			})
		}
		rowNumber++
		totalCount++
	}

	if err := it.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating profiles: %w", err)
	}

	return profiles, totalCount, nil
}

func (h *Handlers) CreateDatasetProfileDownloadHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := parseDatasetRequest(r)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		rowStr := r.URL.Query().Get("row")
		if rowStr == "" {
			httputil.Error(w, errors.New("No row number provided"))
			return
		}
		var rowNum int64
		if _, err := fmt.Sscanf(rowStr, "%d", &rowNum); err != nil {
			httputil.Error(w, errors.Wrap(err, "invalid row parameter"))
			return
		}

		blockMeta, foundDataset, err := h.getDatasetMetadata(r.Context(), req)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		profile, err := h.retrieveProfile(r.Context(), blockMeta, foundDataset, rowNum)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to download profile"))
			return
		}

		timestampStr := time.Unix(0, profile.TimeNanos).UTC().Format("20060102-150405")
		filename := fmt.Sprintf("profile-%s-%s.pb.gz", req.BlockID, timestampStr)
		h.writeProfile(w, profile, filename)
	}
}

func (h *Handlers) retrieveProfile(
	ctx context.Context,
	blockMeta *metastorev1.BlockMeta,
	dataset *metastorev1.Dataset,
	rowNum int64,
) (*googlev1.Profile, error) {
	resolver, timestamp, err := h.buildProfileResolver(ctx, blockMeta, dataset, rowNum)
	if err != nil {
		return nil, fmt.Errorf("failed to build profile resolver: %w", err)
	}
	defer resolver.Release()

	profile, err := resolver.Pprof()
	if err != nil {
		return nil, fmt.Errorf("failed to build pprof profile: %w", err)
	}
	profile.TimeNanos = timestamp

	return profile, nil
}

func (h *Handlers) writeProfile(w http.ResponseWriter, profile *googlev1.Profile, filename string) {
	data, err := profile.MarshalVT()
	if err != nil {
		httputil.Error(w, errors.Wrap(err, "failed to marshal profile"))
		return
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	if _, err := gzipWriter.Write(data); err != nil {
		httputil.Error(w, errors.Wrap(err, "failed to compress profile"))
		return
	}
	if err := gzipWriter.Close(); err != nil {
		httputil.Error(w, errors.Wrap(err, "failed to close gzip writer"))
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	if _, err := w.Write(buf.Bytes()); err != nil {
		httputil.Error(w, errors.Wrap(err, "failed to write profile"))
		return
	}
}

func (h *Handlers) CreateDatasetProfileCallTreeHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := parseDatasetRequest(r)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		rowStr := r.URL.Query().Get("row")
		if rowStr == "" {
			httputil.Error(w, errors.New("No row number provided"))
			return
		}
		var rowNum int64
		if _, err := fmt.Sscanf(rowStr, "%d", &rowNum); err != nil {
			httputil.Error(w, errors.Wrap(err, "invalid row parameter"))
			return
		}

		blockMeta, foundDataset, err := h.getDatasetMetadata(r.Context(), req)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		dataset := h.convertDataset(foundDataset, blockMeta.StringTable)

		tree, timestamp, err := h.buildProfileTree(r.Context(), blockMeta, foundDataset, rowNum)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to build profile tree"))
			return
		}

		err = pageTemplates.profileCallTreeTemplate.Execute(w, profileCallTreePageContent{
			User:        req.TenantID,
			BlockID:     req.BlockID,
			Shard:       req.Shard,
			BlockTenant: req.BlockTenant,
			Dataset:     &dataset,
			Timestamp:   time.Unix(0, timestamp).UTC().Format(time.RFC3339),
			Tree:        tree,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) buildProfileTree(
	ctx context.Context,
	blockMeta *metastorev1.BlockMeta,
	dataset *metastorev1.Dataset,
	rowNum int64,
) (*treeNode, int64, error) {
	resolver, timestamp, err := h.buildProfileResolver(ctx, blockMeta, dataset, rowNum)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build profile resolver: %w", err)
	}
	defer resolver.Release()

	modelTree, err := resolver.Tree()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build tree: %w", err)
	}

	tree := convertModelTreeToTreeNode(modelTree)

	return tree, timestamp, nil
}

func (h *Handlers) buildProfileResolver(
	ctx context.Context,
	blockMeta *metastorev1.BlockMeta,
	dataset *metastorev1.Dataset,
	rowNum int64,
) (*symdb.Resolver, int64, error) {
	obj := block.NewObject(h.Bucket, blockMeta)
	if err := obj.Open(ctx); err != nil {
		return nil, 0, fmt.Errorf("failed to open block object: %w", err)
	}
	defer obj.Close()

	ds := block.NewDataset(dataset, obj)
	if err := ds.Open(ctx, block.SectionProfiles, block.SectionTSDB, block.SectionSymbols); err != nil {
		return nil, 0, fmt.Errorf("failed to open dataset: %w", err)
	}
	defer ds.Close()

	it, err := block.NewProfileRowIterator(ds)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create profile iterator: %w", err)
	}
	defer it.Close()

	var currentRow int64
	var targetEntry block.ProfileEntry
	found := false

	for it.Next() {
		if currentRow == rowNum {
			targetEntry = it.At()
			found = true
			break
		}
		currentRow++
	}

	if err := it.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating profiles: %w", err)
	}

	if !found {
		return nil, 0, fmt.Errorf("profile row %d not found", rowNum)
	}

	resolver := symdb.NewResolver(ctx, ds.Symbols())

	partitionID := targetEntry.Row.StacktracePartitionID()
	var stacktraceIDs []uint32
	var values []uint64

	targetEntry.Row.ForStacktraceIdsAndValues(func(sids []parquet.Value, vals []parquet.Value) {
		stacktraceIDs = make([]uint32, len(sids))
		values = make([]uint64, len(vals))
		for i, sid := range sids {
			stacktraceIDs[i] = sid.Uint32()
		}
		for i, val := range vals {
			values[i] = uint64(val.Int64())
		}
	})

	samples := schemav1.Samples{
		StacktraceIDs: stacktraceIDs,
		Values:        values,
	}
	resolver.AddSamples(partitionID, samples)

	return resolver, targetEntry.Timestamp, nil
}

func convertModelTreeToTreeNode(modelTree *model.Tree) *treeNode {
	if modelTree == nil {
		return nil
	}

	grandTotal := modelTree.Total()
	if grandTotal == 0 {
		return nil
	}

	nodeMap := make(map[string]*treeNode)

	root := &treeNode{
		Name:     "root",
		Value:    uint64(grandTotal),
		Self:     0,
		Percent:  100.0,
		Children: make([]*treeNode, 0),
	}
	nodeMap[""] = root

	modelTree.IterateStacks(func(name string, self int64, stack []string) {
		currentPath := ""
		currentNode := root

		for i := len(stack) - 1; i >= 0; i-- {
			funcName := stack[i]
			parentPath := currentPath
			currentPath = parentPath + "/" + funcName

			node, exists := nodeMap[currentPath]
			if !exists {
				node = &treeNode{
					Name:     funcName,
					Value:    0,
					Self:     0,
					Percent:  0,
					Children: make([]*treeNode, 0),
				}
				nodeMap[currentPath] = node
				currentNode.Children = append(currentNode.Children, node)
			}

			node.Value += uint64(self)

			if i == 0 {
				node.Self += uint64(self)
			}

			currentNode = node
		}
	})

	sortAndCalculatePercents(root, float64(grandTotal))

	return root
}

func sortAndCalculatePercents(node *treeNode, grandTotal float64) {
	if grandTotal > 0 {
		node.Percent = (float64(node.Value) / grandTotal) * 100.0
	}

	if len(node.Children) > 0 {
		slices.SortFunc(node.Children, func(a, b *treeNode) int {
			if a.Value > b.Value {
				return -1
			}
			if a.Value < b.Value {
				return 1
			}
			return 0
		})

		for _, child := range node.Children {
			sortAndCalculatePercents(child, grandTotal)
		}
	}
}

func (h *Handlers) CreateDatasetSymbolsHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := parseDatasetRequest(r)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		blockMeta, foundDataset, err := h.getDatasetMetadata(r.Context(), req)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		dataset := h.convertDataset(foundDataset, blockMeta.StringTable)

		symbolsInfo, err := h.readSymbols(r.Context(), blockMeta, foundDataset)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to read symbols"))
			return
		}

		err = pageTemplates.datasetSymbolsTemplate.Execute(w, datasetSymbolsPageContent{
			User:        req.TenantID,
			BlockID:     req.BlockID,
			Shard:       req.Shard,
			BlockTenant: req.BlockTenant,
			Dataset:     &dataset,
			SymbolsInfo: symbolsInfo,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) readSymbols(ctx context.Context, blockMeta *metastorev1.BlockMeta, dataset *metastorev1.Dataset) (*symbolsInfo, error) {
	obj := block.NewObject(h.Bucket, blockMeta)
	if err := obj.Open(ctx); err != nil {
		return nil, fmt.Errorf("failed to open block object: %w", err)
	}
	defer obj.Close()

	ds := block.NewDataset(dataset, obj)
	if err := ds.Open(ctx, block.SectionSymbols); err != nil {
		return nil, fmt.Errorf("failed to open dataset: %w", err)
	}
	defer ds.Close()

	symbolsReader := ds.Symbols()

	// We need to get a partition to access the actual symbols
	// For v2 blocks, there's typically one partition per dataset
	// We'll use partition 0 as a default
	partitionID := uint64(0)
	partition, err := symbolsReader.Partition(ctx, partitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols partition: %w", err)
	}
	defer partition.Release()

	symbols := partition.Symbols()
	if symbols == nil {
		return nil, fmt.Errorf("symbols partition returned nil")
	}

	// Helper to safely get string from index
	getString := func(idx uint32) string {
		if int(idx) < len(symbols.Strings) {
			return symbols.Strings[idx]
		}
		return fmt.Sprintf("<invalid string id %d>", idx)
	}

	// Collect strings with statistics
	allStrings := symbols.Strings
	totalStrings := len(allStrings)
	var stringEntries []symbolEntry
	maxStrings := 1000

	totalLength := 0
	shortestLen := int(^uint(0) >> 1) // Max int
	longestLen := 0
	var shortestSym, longestSym string
	symbolMap := make(map[string]int)

	for idx, sym := range allStrings {
		symLen := len(sym)
		totalLength += symLen
		symbolMap[sym]++

		if symLen < shortestLen || shortestLen == int(^uint(0)>>1) {
			shortestLen = symLen
			shortestSym = sym
		}
		if symLen > longestLen {
			longestLen = symLen
			longestSym = sym
		}

		if len(stringEntries) < maxStrings {
			stringEntries = append(stringEntries, symbolEntry{
				Index:  idx,
				Symbol: sym,
			})
		}
	}

	avgLength := 0.0
	if totalStrings > 0 {
		avgLength = float64(totalLength) / float64(totalStrings)
	}

	var sampleDuplicates []string
	maxDuplicates := 10
	for sym, count := range symbolMap {
		if count > 1 && len(sampleDuplicates) < maxDuplicates {
			sampleDuplicates = append(sampleDuplicates, fmt.Sprintf("%s (×%d)", sym, count))
		}
	}

	// Collect functions
	var functionEntries []functionEntry
	maxFunctions := 100
	for idx, fn := range symbols.Functions {
		if idx >= maxFunctions {
			break
		}
		functionEntries = append(functionEntries, functionEntry{
			Index:      idx,
			ID:         fn.Id,
			Name:       getString(fn.Name),
			SystemName: getString(fn.SystemName),
			Filename:   getString(fn.Filename),
			StartLine:  fn.StartLine,
		})
	}

	// Collect locations
	var locationEntries []locationEntry
	maxLocations := 100
	for idx, loc := range symbols.Locations {
		if idx >= maxLocations {
			break
		}
		var funcs []string
		for _, line := range loc.Line {
			if int(line.FunctionId) < len(symbols.Functions) {
				fn := symbols.Functions[line.FunctionId]
				funcs = append(funcs, getString(fn.Name))
			}
		}
		locationEntries = append(locationEntries, locationEntry{
			Index:     idx,
			ID:        loc.Id,
			Address:   loc.Address,
			MappingID: loc.MappingId,
			Functions: funcs,
		})
	}

	// Collect mappings
	var mappingEntries []mappingEntry
	maxMappings := 100
	for idx, mapping := range symbols.Mappings {
		if idx >= maxMappings {
			break
		}
		mappingEntries = append(mappingEntries, mappingEntry{
			Index:       idx,
			ID:          mapping.Id,
			MemoryStart: mapping.MemoryStart,
			MemoryLimit: mapping.MemoryLimit,
			FileOffset:  mapping.FileOffset,
			Filename:    getString(mapping.Filename),
			BuildID:     getString(mapping.BuildId),
		})
	}

	// Get stacktrace stats
	var stats symdb.PartitionStats
	partition.WriteStats(&stats)

	return &symbolsInfo{
		Strings:      stringEntries,
		TotalStrings: totalStrings,
		StringStats: symbolsStats{
			TotalLength:      totalLength,
			AverageLength:    avgLength,
			ShortestLength:   shortestLen,
			LongestLength:    longestLen,
			ShortestSymbol:   shortestSym,
			LongestSymbol:    longestSym,
			UniqueSymbols:    len(symbolMap),
			SampleDuplicates: sampleDuplicates,
		},
		Functions:        functionEntries,
		TotalFunctions:   len(symbols.Functions),
		Locations:        locationEntries,
		TotalLocations:   len(symbols.Locations),
		Mappings:         mappingEntries,
		TotalMappings:    len(symbols.Mappings),
		TotalStacktraces: stats.StacktracesTotal,
	}, nil
}
