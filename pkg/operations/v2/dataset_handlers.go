package v2

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

func (h *Handlers) CreateDatasetDetailsHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		tenantId := vars["tenant"]
		if tenantId == "" {
			httputil.Error(w, errors.New("No tenant id provided"))
			return
		}
		blockId := vars["block"]
		if blockId == "" {
			httputil.Error(w, errors.New("No block id provided"))
			return
		}
		datasetName := r.URL.Query().Get("dataset")
		if datasetName == "" {
			httputil.Error(w, errors.New("No dataset name provided"))
			return
		}
		// Handle special case for empty dataset name
		if datasetName == "_empty" {
			datasetName = ""
		}
		shardStr := r.URL.Query().Get("shard")
		if shardStr == "" {
			httputil.Error(w, errors.New("No shard provided"))
			return
		}
		var shard uint32
		if _, err := fmt.Sscanf(shardStr, "%d", &shard); err != nil {
			httputil.Error(w, errors.Wrap(err, "invalid shard parameter"))
			return
		}

		blockTenant := r.URL.Query().Get("block_tenant")

		metadataResp, err := h.MetastoreClient.GetBlockMetadata(r.Context(), &metastorev1.GetBlockMetadataRequest{
			Blocks: &metastorev1.BlockList{
				Tenant: blockTenant,
				Shard:  shard,
				Blocks: []string{blockId},
			},
		})
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to get block metadata"))
			return
		}

		if len(metadataResp.Blocks) == 0 {
			httputil.Error(w, errors.New("Block not found"))
			return
		}

		blockMeta := metadataResp.Blocks[0]

		var foundDataset *metastorev1.Dataset
		for _, ds := range blockMeta.Datasets {
			dsName := blockMeta.StringTable[ds.Name]
			if dsName == datasetName {
				foundDataset = ds
				break
			}
		}

		if foundDataset == nil {
			httputil.Error(w, errors.New("Dataset not found"))
			return
		}

		dataset := h.convertDataset(foundDataset, blockMeta.StringTable)

		err = pageTemplates.datasetDetailsTemplate.Execute(w, datasetDetailsPageContent{
			User:        tenantId,
			BlockID:     blockId,
			Shard:       shard,
			BlockTenant: blockTenant,
			Dataset:     &dataset,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) convertDataset(ds *metastorev1.Dataset, stringTable []string) datasetDetails {
	tenant := stringTable[ds.Tenant]
	datasetName := stringTable[ds.Name]

	var labelSets []labelSet
	pairs := metadata.LabelPairs(ds.Labels)
	for pairs.Next() {
		p := pairs.At()
		var currentSet labelSet
		for len(p) > 0 {
			if len(p) >= 2 {
				key := stringTable[p[0]]
				val := stringTable[p[1]]
				currentSet.Pairs = append(currentSet.Pairs, labelPair{Key: key, Value: val})
				p = p[2:]
			} else {
				break
			}
		}
		if len(currentSet.Pairs) > 0 {
			labelSets = append(labelSets, currentSet)
		}
	}

	var profilesSize, indexSize, symbolsSize uint64
	if len(ds.TableOfContents) >= 3 {
		profilesSize = ds.TableOfContents[1] - ds.TableOfContents[0]
		indexSize = ds.TableOfContents[2] - ds.TableOfContents[1]
		symbolsSize = (ds.TableOfContents[0] + ds.Size) - ds.TableOfContents[2]
	}

	var profilesPercentage, indexPercentage, symbolsPercentage float64
	if ds.Size > 0 {
		profilesPercentage = (float64(profilesSize) / float64(ds.Size)) * 100
		indexPercentage = (float64(indexSize) / float64(ds.Size)) * 100
		symbolsPercentage = (float64(symbolsSize) / float64(ds.Size)) * 100
	}

	return datasetDetails{
		Tenant:             tenant,
		Name:               datasetName,
		MinTime:            msToTime(ds.MinTime).UTC().Format(time.RFC3339),
		MaxTime:            msToTime(ds.MaxTime).UTC().Format(time.RFC3339),
		Size:               humanize.Bytes(ds.Size),
		ProfilesSize:       humanize.Bytes(profilesSize),
		IndexSize:          humanize.Bytes(indexSize),
		SymbolsSize:        humanize.Bytes(symbolsSize),
		ProfilesPercentage: profilesPercentage,
		IndexPercentage:    indexPercentage,
		SymbolsPercentage:  symbolsPercentage,
		LabelSets:          labelSets,
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

		TSDBIndex, err := h.readTSDBIndex(r.Context(), blockMeta, foundDataset)
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
			TSDBIndex:   TSDBIndex,
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

	labels, err := h.getIndexLabels(idx)
	if err != nil {
		return nil, fmt.Errorf("failed to get labels: %w", err)
	}

	symbolIter := idx.Symbols()
	var symbols []string
	for symbolIter.Next() {
		symbols = append(symbols, symbolIter.At())
	}
	if err := symbolIter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate symbols: %w", err)
	}

	series, err := h.getIndexSeries(idx)
	if err != nil {
		return nil, fmt.Errorf("failed to get series: %w", err)
	}

	return &tsdbIndexInfo{
		From:           fromTime,
		Through:        throughTime,
		Checksum:       idx.Checksum(),
		Series:         series,
		Symbols:        symbols,
		LabelValueSets: labels,
	}, nil
}

func (h *Handlers) getIndexLabels(idx phlaredb.IndexReader) ([]labelValueSet, error) {
	labelNames, err := idx.LabelNames()
	if err != nil {
		return nil, fmt.Errorf("failed to get label names: %w", err)
	}
	var labelValueSets []labelValueSet
	for _, labelName := range labelNames {
		values, err := idx.LabelValues(labelName)
		if err != nil {
			return nil, fmt.Errorf("failed to get label values for %s: %w", labelName, err)
		}

		labelValueSets = append(labelValueSets, labelValueSet{
			LabelName:   labelName,
			LabelValues: values,
		})
	}
	return labelValueSets, nil
}

func (h *Handlers) getIndexSeries(idx phlaredb.IndexReader) ([]seriesInfo, error) {
	k2, v2 := index.AllPostingsKey()
	seriesPostings, err := idx.Postings(k2, nil, v2)
	if err != nil {
		return nil, fmt.Errorf("failed to get series postings: %w", err)
	}

	var seriesList []seriesInfo
	var lbls phlaremodel.Labels
	chunks := make([]index.ChunkMeta, 1)

	seriesIdx := uint32(0)
	for seriesPostings.Next() {
		seriesRef := seriesPostings.At()
		_, err := idx.Series(seriesRef, &lbls, &chunks)
		if err != nil {
			return nil, fmt.Errorf("failed to get series %d: %w", seriesRef, err)
		}

		var labelPairs []labelPair
		for _, lbl := range lbls {
			labelPairs = append(labelPairs, labelPair{
				Key:   lbl.Name,
				Value: lbl.Value,
			})
		}

		seriesList = append(seriesList, seriesInfo{
			SeriesIndex: seriesIdx,
			SeriesRef:   uint64(seriesRef),
			Labels:      labelPairs,
		})
		seriesIdx++
	}
	if err := seriesPostings.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate series postings: %w", err)
	}

	return seriesList, nil
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

		symbols, err := h.readSymbols(r.Context(), blockMeta, foundDataset, page, pageSize)
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to read symbols"))
			return
		}

		var totalCount int
		switch tab {
		case "strings":
			totalCount = symbols.TotalStrings
		case "functions":
			totalCount = symbols.TotalFunctions
		case "locations":
			totalCount = symbols.TotalLocations
		case "mappings":
			totalCount = symbols.TotalMappings
		default:
			totalCount = symbols.TotalStrings
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
			Symbols:     symbols,
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

func (h *Handlers) readSymbols(ctx context.Context, blockMeta *metastorev1.BlockMeta, dataset *metastorev1.Dataset, page, pageSize int) (*symbolsInfo, error) {
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

	// NOTE aleks-p: In v2, the partition is always 0.
	// This might change later on, in which case we'll need to retrieve partition IDs from parquet.
	partitionID := uint64(0)
	partition, err := symbolsReader.Partition(ctx, partitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols partition: %w", err)
	}
	defer partition.Release()

	symbols := partition.Symbols()

	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize

	stringEntries := h.getSymbolStrings(symbols.Strings, startIdx, endIdx)
	functionEntries := h.getSymbolFunctions(symbols.Functions, symbols.Strings, startIdx, endIdx)
	locationEntries := h.getSymbolLocations(symbols.Locations, symbols.Functions, symbols.Strings, startIdx, endIdx)
	mappingEntries := h.getSymbolMappings(symbols.Mappings, symbols.Strings, startIdx, endIdx)

	return &symbolsInfo{
		Strings:        stringEntries,
		TotalStrings:   len(symbols.Strings),
		Functions:      functionEntries,
		TotalFunctions: len(symbols.Functions),
		Locations:      locationEntries,
		TotalLocations: len(symbols.Locations),
		Mappings:       mappingEntries,
		TotalMappings:  len(symbols.Mappings),
	}, nil
}

func (h *Handlers) getSymbolStrings(stringTable []string, startIdx int, endIdx int) []symbolEntry {
	stringEntries := make([]symbolEntry, 0, endIdx-startIdx)
	for idx := startIdx; idx < endIdx && idx < len(stringTable); idx++ {
		stringEntries = append(stringEntries, symbolEntry{
			Index:  idx,
			Symbol: stringTable[idx],
		})
	}
	return stringEntries
}

func (h *Handlers) getSymbolFunctions(functions []schemav1.InMemoryFunction, stringTable []string, startIdx int, endIdx int) []functionEntry {
	functionEntries := make([]functionEntry, 0, endIdx-startIdx)
	for idx := startIdx; idx < endIdx && idx < len(functions); idx++ {
		fn := functions[idx]
		functionEntries = append(functionEntries, functionEntry{
			Index:      idx,
			ID:         fn.Id,
			Name:       stringTable[fn.Name],
			SystemName: stringTable[fn.SystemName],
			Filename:   stringTable[fn.Filename],
			StartLine:  fn.StartLine,
		})
	}
	return functionEntries
}

func (h *Handlers) getSymbolLocations(
	locations []schemav1.InMemoryLocation,
	functions []schemav1.InMemoryFunction,
	stringTable []string,
	startIdx int, endIdx int,
) []locationEntry {
	locationEntries := make([]locationEntry, 0, endIdx-startIdx)
	for idx := startIdx; idx < endIdx && idx < len(locations); idx++ {
		loc := locations[idx]
		var lines []locationLine
		for _, line := range loc.Line {
			fn := functions[line.FunctionId]
			lines = append(lines, locationLine{
				FunctionName: stringTable[fn.Name],
				Line:         int64(line.Line),
			})
		}
		locationEntries = append(locationEntries, locationEntry{
			Index:     idx,
			ID:        loc.Id,
			Address:   loc.Address,
			MappingID: loc.MappingId,
			Lines:     lines,
		})
	}
	return locationEntries
}

func (h *Handlers) getSymbolMappings(mappings []schemav1.InMemoryMapping, stringTable []string, startIdx int, endIdx int) []mappingEntry {
	mappingEntries := make([]mappingEntry, 0, endIdx-startIdx)
	for idx := startIdx; idx < endIdx && idx < len(mappings); idx++ {
		mapping := mappings[idx]
		mappingEntries = append(mappingEntries, mappingEntry{
			Index:       idx,
			ID:          mapping.Id,
			MemoryStart: mapping.MemoryStart,
			MemoryLimit: mapping.MemoryLimit,
			FileOffset:  mapping.FileOffset,
			Filename:    stringTable[mapping.Filename],
			BuildID:     stringTable[mapping.BuildId],
		})
	}
	return mappingEntries
}
