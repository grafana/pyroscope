package v2

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

func (h *Handlers) CreateDatasetDetailsHandler() func(http.ResponseWriter, *http.Request) {
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

		err = pageTemplates.datasetDetailsTemplate.Execute(w, datasetDetailsPageContent{
			User:        req.TenantID,
			BlockID:     req.BlockID,
			Shard:       req.Shard,
			BlockTenant: req.BlockTenant,
			Dataset:     &dataset,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) CreateDatasetTSDBIndexHandler() func(http.ResponseWriter, *http.Request) {
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

		indexInfo, err := h.readTSDBIndex(r.Context(), blockMeta, foundDataset)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to read TSDB index"))
			return
		}

		err = pageTemplates.datasetIndexTemplate.Execute(w, datasetIndexPageContent{
			User:        req.TenantID,
			BlockID:     req.BlockID,
			Shard:       req.Shard,
			BlockTenant: req.BlockTenant,
			Dataset:     &dataset,
			IndexInfo:   indexInfo,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) readTSDBIndex(ctx context.Context, blockMeta *metastorev1.BlockMeta, dataset *metastorev1.Dataset) (*tsdbIndexInfo, error) {
	obj := block.NewObject(h.Bucket, blockMeta)
	if err := obj.Open(ctx); err != nil {
		return nil, fmt.Errorf("failed to open block object: %w", err)
	}
	defer obj.Close()

	ds := block.NewDataset(dataset, obj)
	if err := ds.Open(ctx, block.SectionTSDB); err != nil {
		return nil, fmt.Errorf("failed to open dataset: %w", err)
	}
	defer ds.Close()

	idx := ds.Index()

	from, through := idx.Bounds()
	fromTime := time.Unix(0, from).UTC().Format(time.RFC3339)
	throughTime := time.Unix(0, through).UTC().Format(time.RFC3339)

	checksum := idx.Checksum()

	k, v := index.AllPostingsKey()
	postings, err := idx.Postings(k, nil, v)
	if err != nil {
		return nil, fmt.Errorf("failed to get all postings: %w", err)
	}
	var numSeries uint64
	for postings.Next() {
		numSeries++
	}
	if err := postings.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate postings: %w", err)
	}

	labelNames, err := idx.LabelNames()
	if err != nil {
		return nil, fmt.Errorf("failed to get label names: %w", err)
	}

	symbolIter := idx.Symbols()
	var symbols []string
	maxSymbols := 100
	for symbolIter.Next() && len(symbols) < maxSymbols {
		symbols = append(symbols, symbolIter.At())
	}
	if err := symbolIter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate symbols: %w", err)
	}

	totalSymbols := len(symbols)
	for symbolIter.Next() {
		totalSymbols++
	}

	var labelValueSets []labelValueSet
	maxLabelNames := 20
	for i, labelName := range labelNames {
		if i >= maxLabelNames {
			break
		}

		values, err := idx.LabelValues(labelName)
		if err != nil {
			return nil, fmt.Errorf("failed to get label values for %s: %w", labelName, err)
		}

		maxValues := 20
		sampleValues := values
		if len(sampleValues) > maxValues {
			sampleValues = sampleValues[:maxValues]
		}

		labelValueSets = append(labelValueSets, labelValueSet{
			LabelName:    labelName,
			NumValues:    len(values),
			SampleValues: sampleValues,
		})
	}

	return &tsdbIndexInfo{
		From:           fromTime,
		Through:        throughTime,
		Checksum:       checksum,
		NumSeries:      numSeries,
		LabelNames:     labelNames,
		NumSymbols:     len(symbols),
		SampleSymbols:  symbols,
		TotalSymbols:   totalSymbols,
		LabelValueSets: labelValueSets,
	}, nil
}
