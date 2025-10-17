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

			profileType := entry.Labels.Get(phlaremodel.LabelNameProfileType)

			var sampleCount int
			entry.Row.ForStacktraceIdsAndValues(func(sids []parquet.Value, vals []parquet.Value) {
				sampleCount = len(sids)
			})

			profiles = append(profiles, profileInfo{
				RowNumber:   rowNumber,
				Timestamp:   time.Unix(0, entry.Timestamp).UTC().Format(time.RFC3339),
				SeriesIndex: entry.Row.SeriesIndex(),
				ProfileType: profileType,
				SampleCount: sampleCount,
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

	var labelPairs []labelPair
	for _, label := range targetEntry.Labels {
		labelPairs = append(labelPairs, labelPair{
			Key:   label.Name,
			Value: label.Value,
		})
	}

	var sampleCount int
	targetEntry.Row.ForStacktraceIdsAndValues(func(sids []parquet.Value, vals []parquet.Value) {
		sampleCount = len(sids)
	})

	profileMeta := &profileMetadata{
		Labels:      labelPairs,
		SampleCount: sampleCount,
		Unit:        targetEntry.Labels.Get(phlaremodel.LabelNameUnit),
		ProfileType: targetEntry.Labels.Get(phlaremodel.LabelNameProfileType),
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
	scaledValue, scaledUnit := measurement.Scale(int64(value), unit, "auto")
	formattedValue := strings.TrimSuffix(fmt.Sprintf("%.2f", scaledValue), ".00")
	if scaledUnit == "" {
		return formattedValue
	}
	return fmt.Sprintf("%s %s", formattedValue, scaledUnit)
}

func buildTreeFromPprof(profile *googlev1.Profile, unit string) *treeNode {
	if profile == nil || len(profile.Sample) == 0 {
		return nil
	}

	var grandTotal uint64
	for _, sample := range profile.Sample {
		if len(sample.Value) > 0 {
			grandTotal += uint64(sample.Value[0])
		}
	}

	if grandTotal == 0 {
		return nil
	}

	functionMap := make(map[uint64]*googlev1.Function)
	for _, fn := range profile.Function {
		functionMap[fn.Id] = fn
	}

	locationMap := make(map[uint64]*googlev1.Location)
	for _, loc := range profile.Location {
		locationMap[loc.Id] = loc
	}

	root := &treeNode{
		Name:           "root",
		Value:          grandTotal,
		Percent:        100.0,
		Location:       "",
		FormattedValue: formatValue(grandTotal, unit),
		Children:       make([]*treeNode, 0),
	}

	nodeMap := make(map[string]*treeNode)
	nodeMap[""] = root

	for _, sample := range profile.Sample {
		if len(sample.Value) == 0 || len(sample.LocationId) == 0 {
			continue
		}

		value := uint64(sample.Value[0])
		currentPath := ""
		currentNode := root

		for i := len(sample.LocationId) - 1; i >= 0; i-- {
			locID := sample.LocationId[i]
			location := locationMap[locID]
			if location == nil {
				continue
			}

			if len(location.Line) == 0 {
				continue
			}

			line := location.Line[0]
			function := functionMap[line.FunctionId]
			if function == nil {
				continue
			}

			funcName := profile.StringTable[function.Name]
			fileName := profile.StringTable[function.Filename]
			lineNum := line.Line

			var locationStr string
			if fileName != "" && lineNum > 0 {
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

			parentPath := currentPath
			currentPath = fmt.Sprintf("%s/%s@%d", parentPath, funcName, line.FunctionId)

			node, exists := nodeMap[currentPath]
			if !exists {
				node = &treeNode{
					Name:           funcName,
					Percent:        0,
					Location:       locationStr,
					FormattedValue: formatValue(0, unit),
					Children:       make([]*treeNode, 0),
				}
				nodeMap[currentPath] = node
				currentNode.Children = append(currentNode.Children, node)
			}

			node.Value += value

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

	node.FormattedValue = formatValue(node.Value, unit)

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
