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

		blockMeta, foundDataset, err := h.getDatasetMetadata(r.Context(), req)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		dataset := h.convertDataset(foundDataset, blockMeta.StringTable)

		// Read actual profiles from the block
		profiles, err := h.readProfilesFromDataset(r.Context(), blockMeta, foundDataset)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to read profiles from dataset"))
			return
		}

		err = pageTemplates.datasetProfilesTemplate.Execute(w, datasetProfilesPageContent{
			User:        req.TenantID,
			BlockID:     req.BlockID,
			Shard:       req.Shard,
			BlockTenant: req.BlockTenant,
			Dataset:     &dataset,
			Profiles:    profiles,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) readProfilesFromDataset(ctx context.Context, blockMeta *metastorev1.BlockMeta, dataset *metastorev1.Dataset) ([]profileInfo, error) {
	obj := block.NewObject(h.Bucket, blockMeta)
	if err := obj.Open(ctx); err != nil {
		return nil, fmt.Errorf("failed to open block object: %w", err)
	}
	defer obj.Close()

	ds := block.NewDataset(dataset, obj)
	if err := ds.Open(ctx, block.SectionProfiles, block.SectionTSDB); err != nil {
		return nil, fmt.Errorf("failed to open dataset: %w", err)
	}
	defer ds.Close()

	it, err := block.NewProfileRowIterator(ds)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile iterator: %w", err)
	}
	defer it.Close()

	var profiles []profileInfo
	rowNumber := 0
	maxProfiles := 1000

	for it.Next() && rowNumber < maxProfiles {
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
		rowNumber++
	}

	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("error iterating profiles: %w", err)
	}

	return profiles, nil
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

func (h *Handlers) CreateDatasetProfileVisualizeHandler() func(http.ResponseWriter, *http.Request) {
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

		err = pageTemplates.profileVisualizationTemplate.Execute(w, profileVisualizationPageContent{
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
