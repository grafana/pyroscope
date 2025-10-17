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
	"github.com/grafana/pyroscope/pkg/frontend/dot/measurement"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/pprof"
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

			// Extract profile type from labels
			var profileType string
			for _, label := range entry.Labels {
				if label.Name == "__profile_type__" {
					profileType = label.Value
				}
			}

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
				ProfileType: profileType,
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

		// Get profile metadata to extract profile type
		_, _, profileMeta, err := h.buildProfileResolver(r.Context(), blockMeta, foundDataset, rowNum)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to get profile metadata"))
			return
		}

		profile, err := h.retrieveProfile(r.Context(), blockMeta, foundDataset, rowNum)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to download profile"))
			return
		}

		// Sanitize dataset name and profile type for filename
		// Replace slashes and colons with underscores
		sanitizedDataset := strings.ReplaceAll(req.DatasetName, "/", "_")
		sanitizedProfileType := strings.ReplaceAll(profileMeta.ProfileType, ":", "_")
		sanitizedProfileType = strings.ReplaceAll(sanitizedProfileType, "/", "_")

		timestampStr := time.Unix(0, profile.TimeNanos).UTC().Format("20060102-150405")
		filename := fmt.Sprintf("%s-%s-%s.pb.gz", sanitizedDataset, sanitizedProfileType, timestampStr)
		h.writeProfile(w, profile, filename)
	}
}

func (h *Handlers) retrieveProfile(
	ctx context.Context,
	blockMeta *metastorev1.BlockMeta,
	dataset *metastorev1.Dataset,
	rowNum int64,
) (*googlev1.Profile, error) {
	resolver, timestamp, meta, err := h.buildProfileResolver(ctx, blockMeta, dataset, rowNum)
	if err != nil {
		return nil, fmt.Errorf("failed to build profile resolver: %w", err)
	}
	defer resolver.Release()

	profile, err := resolver.Pprof()
	if err != nil {
		return nil, fmt.Errorf("failed to build pprof profile: %w", err)
	}

	if t, err := phlaremodel.ParseProfileTypeSelector(meta.ProfileType); err == nil {
		pprof.SetProfileMetadata(profile, t, timestamp, 0)
	}

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

		tree, timestamp, profileMeta, err := h.buildProfileTree(r.Context(), blockMeta, foundDataset, rowNum)
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
			ProfileInfo: profileMeta,
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
) (*treeNode, int64, *profileMetadata, error) {
	resolver, timestamp, profileMeta, err := h.buildProfileResolver(ctx, blockMeta, dataset, rowNum)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to build profile resolver: %w", err)
	}
	defer resolver.Release()

	profile, err := resolver.Pprof()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to build pprof profile: %w", err)
	}

	tree := buildTreeFromPprof(profile, profileMeta.Unit)

	return tree, timestamp, profileMeta, nil
}

func (h *Handlers) buildProfileResolver(
	ctx context.Context,
	blockMeta *metastorev1.BlockMeta,
	dataset *metastorev1.Dataset,
	rowNum int64,
) (*symdb.Resolver, int64, *profileMetadata, error) {
	obj := block.NewObject(h.Bucket, blockMeta)
	if err := obj.Open(ctx); err != nil {
		return nil, 0, nil, fmt.Errorf("failed to open block object: %w", err)
	}
	defer obj.Close()

	ds := block.NewDataset(dataset, obj)
	if err := ds.Open(ctx, block.SectionProfiles, block.SectionTSDB, block.SectionSymbols); err != nil {
		return nil, 0, nil, fmt.Errorf("failed to open dataset: %w", err)
	}
	defer ds.Close()

	it, err := block.NewProfileRowIterator(ds)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to create profile iterator: %w", err)
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
		return nil, 0, nil, fmt.Errorf("error iterating profiles: %w", err)
	}

	if !found {
		return nil, 0, nil, fmt.Errorf("profile row %d not found", rowNum)
	}

	// Build profile metadata with labels
	var labelPairs []labelPair
	var unit string
	var profileType string
	for _, label := range targetEntry.Labels {
		labelPairs = append(labelPairs, labelPair{
			Key:   label.Name,
			Value: label.Value,
		})
		// Extract the unit label
		if label.Name == "__unit__" {
			unit = label.Value
		}
		// Extract the profile type label
		if label.Name == "__profile_type__" {
			profileType = label.Value
		}
	}

	var sampleCount int
	var totalValue uint64
	targetEntry.Row.ForStacktraceIdsAndValues(func(sids []parquet.Value, vals []parquet.Value) {
		sampleCount = len(sids)
		for _, val := range vals {
			totalValue += uint64(val.Int64())
		}
	})

	profileMeta := &profileMetadata{
		Labels:      labelPairs,
		TotalValue:  totalValue,
		SampleCount: sampleCount,
		Fingerprint: uint64(targetEntry.Fingerprint),
		Unit:        unit,
		ProfileType: profileType,
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

	return resolver, targetEntry.Timestamp, profileMeta, nil
}

// formatValue formats a value according to the pprof unit specification
func formatValue(value uint64, unit string) string {
	// Use the measurement package to format the value
	scaledValue, scaledUnit := measurement.Scale(int64(value), unit, "auto")

	// Format the value with 2 decimal places, removing trailing zeros
	formattedValue := strings.TrimSuffix(fmt.Sprintf("%.2f", scaledValue), ".00")

	// Combine value and unit
	if scaledUnit == "" {
		return formattedValue
	}
	return fmt.Sprintf("%s %s", formattedValue, scaledUnit)
}

func buildTreeFromPprof(profile *googlev1.Profile, unit string) *treeNode {
	if profile == nil || len(profile.Sample) == 0 {
		return nil
	}

	// Calculate total value
	var grandTotal uint64
	for _, sample := range profile.Sample {
		if len(sample.Value) > 0 {
			grandTotal += uint64(sample.Value[0])
		}
	}

	if grandTotal == 0 {
		return nil
	}

	// Build string table for quick lookup
	getString := func(idx int64) string {
		if idx >= 0 && int(idx) < len(profile.StringTable) {
			return profile.StringTable[idx]
		}
		return ""
	}

	// Build function and location maps
	functionMap := make(map[uint64]*googlev1.Function)
	for _, fn := range profile.Function {
		functionMap[fn.Id] = fn
	}

	locationMap := make(map[uint64]*googlev1.Location)
	for _, loc := range profile.Location {
		locationMap[loc.Id] = loc
	}

	// Create root node
	root := &treeNode{
		Name:           "root",
		Value:          grandTotal,
		Self:           0,
		Percent:        100.0,
		Location:       "",
		FullPath:       "",
		LineNumber:     0,
		FormattedValue: formatValue(grandTotal, unit),
		FormattedSelf:  formatValue(0, unit),
		Children:       make([]*treeNode, 0),
	}

	// Track nodes by path to merge duplicate stacks
	nodeMap := make(map[string]*treeNode)
	nodeMap[""] = root

	// Process each sample
	for _, sample := range profile.Sample {
		if len(sample.Value) == 0 || len(sample.LocationId) == 0 {
			continue
		}

		value := uint64(sample.Value[0])
		currentPath := ""
		currentNode := root

		// Walk the stack from root (reversed order)
		for i := len(sample.LocationId) - 1; i >= 0; i-- {
			locID := sample.LocationId[i]
			location := locationMap[locID]
			if location == nil {
				continue
			}

			// Get the first function at this location (innermost)
			if len(location.Line) == 0 {
				continue
			}

			line := location.Line[0]
			function := functionMap[line.FunctionId]
			if function == nil {
				continue
			}

			funcName := getString(function.Name)
			fileName := getString(function.Filename)
			lineNum := line.Line

			// Build location string and store full path
			var locationStr string
			var fullPath string
			var lineNumber int64
			if fileName != "" && lineNum > 0 {
				fullPath = fileName
				lineNumber = lineNum

				// Simplify file path for display
				filePath := fileName
				if pkgIdx := strings.Index(filePath, "/pkg/"); pkgIdx != -1 {
					filePath = filePath[pkgIdx+1:]
				} else if srcIdx := strings.Index(filePath, "/src/"); srcIdx != -1 {
					filePath = filePath[srcIdx+5:]
				} else if pathIdx := strings.LastIndex(filePath, "/"); pathIdx != -1 {
					filePath = filePath[pathIdx+1:]
				}
				locationStr = fmt.Sprintf("%s:L%d", filePath, lineNum)
			}

			// Create unique path for this node
			parentPath := currentPath
			currentPath = fmt.Sprintf("%s/%s@%d", parentPath, funcName, line.FunctionId)

			node, exists := nodeMap[currentPath]
			if !exists {
				node = &treeNode{
					Name:           funcName,
					Value:          0,
					Self:           0,
					Percent:        0,
					Location:       locationStr,
					FullPath:       fullPath,
					LineNumber:     lineNumber,
					FormattedValue: formatValue(0, unit),
					FormattedSelf:  formatValue(0, unit),
					Children:       make([]*treeNode, 0),
				}
				nodeMap[currentPath] = node
				currentNode.Children = append(currentNode.Children, node)
			}

			node.Value += value

			// If this is the leaf node, add to self
			if i == 0 {
				node.Self += value
			}

			currentNode = node
		}
	}

	sortAndCalculatePercents(root, float64(grandTotal), unit)

	return root
}

func sortAndCalculatePercents(node *treeNode, grandTotal float64, unit string) {
	if grandTotal > 0 {
		node.Percent = (float64(node.Value) / grandTotal) * 100.0
	}

	// Update formatted values after all accumulation is done
	node.FormattedValue = formatValue(node.Value, unit)
	node.FormattedSelf = formatValue(node.Self, unit)

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
			sortAndCalculatePercents(child, grandTotal, unit)
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

		tab := r.URL.Query().Get("tab")
		if tab == "" {
			tab = "strings"
		}

		blockMeta, foundDataset, err := h.getDatasetMetadata(r.Context(), req)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		dataset := h.convertDataset(foundDataset, blockMeta.StringTable)

		symbolsInfo, err := h.readSymbols(r.Context(), blockMeta, foundDataset, tab, page, pageSize)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to read symbols"))
			return
		}

		// Calculate total pages based on the active tab
		var totalCount int
		switch tab {
		case "strings":
			totalCount = symbolsInfo.TotalStrings
		case "functions":
			totalCount = symbolsInfo.TotalFunctions
		case "locations":
			totalCount = symbolsInfo.TotalLocations
		case "mappings":
			totalCount = symbolsInfo.TotalMappings
		default:
			totalCount = symbolsInfo.TotalStrings
		}

		totalPages := (totalCount + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}

		err = pageTemplates.datasetSymbolsTemplate.Execute(w, datasetSymbolsPageContent{
			User:        req.TenantID,
			BlockID:     req.BlockID,
			Shard:       req.Shard,
			BlockTenant: req.BlockTenant,
			Dataset:     &dataset,
			SymbolsInfo: symbolsInfo,
			Page:        page,
			PageSize:    pageSize,
			TotalPages:  totalPages,
			HasPrevPage: page > 1,
			HasNextPage: page < totalPages,
			Tab:         tab,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) readSymbols(ctx context.Context, blockMeta *metastorev1.BlockMeta, dataset *metastorev1.Dataset, tab string, page, pageSize int) (*symbolsInfo, error) {
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

	// Get counts for tab headers
	allStrings := symbols.Strings
	totalStrings := len(allStrings)

	// Paginate based on the active tab
	var stringEntries []symbolEntry
	var functionEntries []functionEntry
	var locationEntries []locationEntry
	var mappingEntries []mappingEntry

	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize

	switch tab {
	case "strings":
		if endIdx > totalStrings {
			endIdx = totalStrings
		}
		for idx := startIdx; idx < endIdx; idx++ {
			stringEntries = append(stringEntries, symbolEntry{
				Index:  idx,
				Symbol: allStrings[idx],
			})
		}

	case "functions":
		totalFunctions := len(symbols.Functions)
		if endIdx > totalFunctions {
			endIdx = totalFunctions
		}
		for idx := startIdx; idx < endIdx; idx++ {
			fn := symbols.Functions[idx]
			functionEntries = append(functionEntries, functionEntry{
				Index:      idx,
				ID:         fn.Id,
				Name:       getString(fn.Name),
				SystemName: getString(fn.SystemName),
				Filename:   getString(fn.Filename),
				StartLine:  fn.StartLine,
			})
		}

	case "locations":
		totalLocations := len(symbols.Locations)
		if endIdx > totalLocations {
			endIdx = totalLocations
		}
		for idx := startIdx; idx < endIdx; idx++ {
			loc := symbols.Locations[idx]
			var lines []locationLine
			for _, line := range loc.Line {
				if int(line.FunctionId) < len(symbols.Functions) {
					fn := symbols.Functions[line.FunctionId]
					lines = append(lines, locationLine{
						FunctionName: getString(fn.Name),
						Line:         int64(line.Line),
					})
				}
			}
			locationEntries = append(locationEntries, locationEntry{
				Index:     idx,
				ID:        loc.Id,
				Address:   loc.Address,
				MappingID: loc.MappingId,
				Lines:     lines,
			})
		}

	case "mappings":
		totalMappings := len(symbols.Mappings)
		if endIdx > totalMappings {
			endIdx = totalMappings
		}
		for idx := startIdx; idx < endIdx; idx++ {
			mapping := symbols.Mappings[idx]
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
	}

	// Get stacktrace stats
	var stats symdb.PartitionStats
	partition.WriteStats(&stats)

	return &symbolsInfo{
		Strings:          stringEntries,
		TotalStrings:     totalStrings,
		Functions:        functionEntries,
		TotalFunctions:   len(symbols.Functions),
		Locations:        locationEntries,
		TotalLocations:   len(symbols.Locations),
		Mappings:         mappingEntries,
		TotalMappings:    len(symbols.Mappings),
		TotalStacktraces: stats.StacktracesTotal,
	}, nil
}
