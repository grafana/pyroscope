package compactionworker

import (
	"context"
	"crypto/rand"
	"flag"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"
	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util/build"
)

type Worker struct {
	*services.BasicService

	config          Config
	logger          log.Logger
	metastoreClient *metastoreclient.Client
	storage         objstore.Bucket

	jobMutex             sync.RWMutex
	pendingJobs          map[string]*compactorv1.CompactionJob
	activeJobs           map[string]*compactorv1.CompactionJob
	pendingStatusUpdates map[string]*compactorv1.CompactionJobStatus
}

type Config struct {
	JobCapacity int `yaml:"job_capacity"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker."
	f.IntVar(&cfg.JobCapacity, prefix+"job-capacity", 5, "how many concurrent jobs will a worker run at most")
}

func New(config Config, logger log.Logger, metastoreClient *metastoreclient.Client, storage objstore.Bucket) (*Worker, error) {
	w := &Worker{
		config:               config,
		logger:               logger,
		metastoreClient:      metastoreClient,
		storage:              storage,
		pendingJobs:          make(map[string]*compactorv1.CompactionJob),
		activeJobs:           make(map[string]*compactorv1.CompactionJob),
		pendingStatusUpdates: make(map[string]*compactorv1.CompactionJobStatus),
	}
	w.BasicService = services.NewBasicService(w.starting, w.running, w.stopping)
	return w, nil
}

func (w *Worker) starting(ctx context.Context) (err error) {
	return nil
}

func (w *Worker) running(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.poll(ctx)

			w.jobMutex.RLock()
			pendingJobs := make(map[string]*compactorv1.CompactionJob, len(w.pendingJobs))
			for _, job := range w.pendingJobs {
				pendingJobs[job.Name] = job
			}
			w.jobMutex.RUnlock()

			if len(pendingJobs) > 0 {
				level.Info(w.logger).Log("msg", "starting pending compaction jobs", "pendingJobs", len(pendingJobs))
				for _, job := range pendingJobs {
					job := job
					go func() {
						w.jobMutex.Lock()
						w.activeJobs[job.Name] = job
						delete(w.pendingJobs, job.Name)
						w.jobMutex.Unlock()

						level.Info(w.logger).Log("msg", "starting compaction job", "job", job.Name)
						status := w.startJob(ctx, job)

						level.Info(w.logger).Log("msg", "compaction job finished", "job", job.Name)

						w.jobMutex.Lock()
						w.pendingStatusUpdates[job.Name] = status
						delete(w.activeJobs, job.Name)
						w.jobMutex.Unlock()
					}()
				}
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	w.jobMutex.Lock()
	level.Debug(w.logger).Log(
		"msg", "polling for compaction jobs and status updates",
		"active_jobs", len(w.activeJobs),
		"pending_jobs", len(w.pendingJobs),
		"pending_updates", len(w.pendingStatusUpdates))

	pendingStatusUpdates := make([]*compactorv1.CompactionJobStatus, 0, len(w.pendingStatusUpdates))
	for _, update := range w.pendingStatusUpdates {
		level.Debug(w.logger).Log("msg", "pending compaction job update", "job", update.JobName, "status", update.Status)
		pendingStatusUpdates = append(pendingStatusUpdates, update)
	}
	for _, activeJob := range w.activeJobs {
		level.Debug(w.logger).Log("msg", "in progress job update", "job", activeJob.Name)
		pendingStatusUpdates = append(pendingStatusUpdates, &compactorv1.CompactionJobStatus{
			JobName:      activeJob.Name,
			Status:       compactorv1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS,
			RaftLogIndex: activeJob.RaftLogIndex,
			Shard:        activeJob.Shard,
			TenantId:     activeJob.TenantId,
		})
	}
	jobCapacity := uint32(w.config.JobCapacity - len(w.activeJobs) - len(w.pendingJobs))
	w.jobMutex.Unlock()

	if len(pendingStatusUpdates) > 0 || jobCapacity > 0 {
		jobsResponse, err := w.metastoreClient.PollCompactionJobs(ctx, &compactorv1.PollCompactionJobsRequest{
			JobStatusUpdates: pendingStatusUpdates,
			JobCapacity:      jobCapacity,
		})

		if err != nil {
			level.Error(w.logger).Log("msg", "failed to poll compaction jobs", "err", err)
			return
		}

		level.Debug(w.logger).Log("msg", "poll response received", "compaction_jobs", len(jobsResponse.CompactionJobs))

		w.jobMutex.Lock()
		for _, update := range pendingStatusUpdates {
			delete(w.pendingStatusUpdates, update.JobName)
		}

		for _, pendingJob := range jobsResponse.CompactionJobs {
			w.pendingJobs[pendingJob.Name] = pendingJob
		}
		w.jobMutex.Unlock()
	}
}

func (w *Worker) stopping(err error) error {
	// TODO aleks: handle shutdown
	return nil
}

func (w *Worker) startJob(ctx context.Context, job *compactorv1.CompactionJob) *compactorv1.CompactionJobStatus {
	jobStatus := &compactorv1.CompactionJobStatus{
		JobName:      job.Name,
		CompletedJob: &compactorv1.CompletedJob{},
		Shard:        job.Shard,
		TenantId:     job.TenantId,
		RaftLogIndex: job.RaftLogIndex,
	}

	level.Info(w.logger).Log(
		"msg", "compacting blocks for job",
		"job", job.Name,
		"blocks", len(job.Blocks))

	compactedBlockMetas, err := w.compactBlocks(ctx, job.Name, job.Blocks)
	if err != nil {
		level.Error(w.logger).Log("msg", "failed to run block compaction", "err", err, "job", job.Name)
		jobStatus.Status = compactorv1.CompactionStatus_COMPACTION_STATUS_FAILURE
		return jobStatus
	}

	level.Info(w.logger).Log(
		"msg", "successful compaction for job",
		"job", job.Name,
		"blocks", len(job.Blocks),
		"output_blocks", len(compactedBlockMetas))

	jobStatus.Status = compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS
	jobStatus.CompletedJob.Blocks = compactedBlockMetas

	return jobStatus
}

type serviceReader struct {
	meta           *metastorev1.BlockMeta
	svcMeta        *metastorev1.TenantService
	indexReader    *index.Reader
	symbolsReader  *symdb.Reader
	profilesReader *parquet.File
}

func newServiceReader(meta *metastorev1.BlockMeta, svcMeta *metastorev1.TenantService, indexReader *index.Reader, symbolsReader *symdb.Reader, profilesReader *parquet.File) *serviceReader {
	return &serviceReader{
		meta:           meta,
		svcMeta:        svcMeta,
		indexReader:    indexReader,
		symbolsReader:  symbolsReader,
		profilesReader: profilesReader,
	}
}

type ProfileReader interface {
	io.ReaderAt
	Schema() *parquet.Schema
	Root() *parquet.Column
	RowGroups() []parquet.RowGroup
}

func (w *Worker) compactBlocks(ctx context.Context, jobName string, blocks []*metastorev1.BlockMeta) ([]*metastorev1.BlockMeta, error) {
	// download blocks
	shard := blocks[0].Shard
	compactionLevel := blocks[0].CompactionLevel + 1

	// create queriers (block readers) from blocks
	svcReaders := make(map[string][]*serviceReader)
	for _, b := range blocks {
		o := newObject(w.storage, b)
		for _, svc := range b.TenantServices {
			i, err := o.openTsdb(ctx, svc)
			if err != nil {
				return nil, errors.Wrap(err, "failed to open tsdb index")
			}
			sdb, err := o.openSymdb(ctx, svc)
			if err != nil {
				return nil, errors.Wrap(err, "failed to open symbol db")
			}
			profiles, err := o.openProfileTable(ctx, svc)
			if err != nil {
				return nil, errors.Wrap(err, "failed to open profile table")
			}
			svcReaders[svc.TenantId] = append(svcReaders[svc.TenantId], newServiceReader(b, svc, i, sdb, profiles))
		}
	}

	writers := make(map[string]*blockWriter)
	for tenant, _ := range svcReaders {
		dest := filepath.Join("data-compactor", tenant, jobName)
		writer, err := w.createBlockWriter(dest, tenant, shard, compactionLevel)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create block writer")
		}
		writers[tenant] = writer
	}

	// compact
	metas := make([]*metastorev1.BlockMeta, 0, len(svcReaders))
	for tenant, readers := range svcReaders {
		meta, err := writers[tenant].compact(ctx, readers)
		if err != nil {
			return nil, err
		}
		metas = append(metas, meta)
	}

	// upload blocks
	for _, meta := range metas {
		if err := w.uploadBlock(ctx, jobName, meta); err != nil {
			return nil, err
		}
	}

	return metas, nil
}

func (w *Worker) uploadBlock(ctx context.Context, jobName string, meta *metastorev1.BlockMeta) error {
	blockPath := filepath.Join("data-compactor", meta.TenantId, jobName, "block.bin")
	file, err := os.Open(blockPath)
	if err != nil {
		return errors.Wrap(err, "failed to open compacted block")
	}
	defer file.Close()

	o := newObject(w.storage, meta)
	return w.storage.Upload(ctx, o.path, file)
}

type serviceReaderGroup struct {
	svc     string
	readers []*serviceReader
}

func (bw *blockWriter) compact(ctx context.Context, readers []*serviceReader) (*metastorev1.BlockMeta, error) {
	// group by tenant service
	readersByService := make(map[string][]*serviceReader)
	for _, reader := range readers {
		readersByService[reader.svcMeta.Name] = append(readersByService[reader.svcMeta.Name], reader)
	}

	// sort by tenant service
	serviceGroups := make([]*serviceReaderGroup, 0, len(readersByService))
	for svc, r := range readersByService {
		serviceGroups = append(serviceGroups, &serviceReaderGroup{
			svc:     svc,
			readers: r,
		})
	}

	// prepare output file
	blockFile, err := os.Create(filepath.Join(bw.path, "block.bin"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to open block file")
	}
	defer blockFile.Close()
	offset := uint64(0)

	minTime := int64(math.MaxInt64)
	maxTime := int64(math.MinInt64)
	for _, group := range serviceGroups {
		minTimeSvc := int64(math.MaxInt64)
		maxTimeSvc := int64(math.MinInt64)
		for _, reader := range group.readers {
			it, err := newProfileRowIterator(reader) // TODO aleks: we probably want to sort profiles here
			if err != nil {
				return nil, err
			}
			for it.Next() {
				p := it.At()

				err := bw.WriteRow(group.svc, p)
				if err != nil {
					return nil, err
				}
			}

			if reader.svcMeta.MinTime < minTimeSvc {
				minTimeSvc = reader.svcMeta.MinTime
			}
			if reader.svcMeta.MaxTime > maxTimeSvc {
				maxTimeSvc = reader.svcMeta.MaxTime
			}
		}
		if minTimeSvc < minTime {
			minTime = minTimeSvc
		}
		if maxTimeSvc > maxTime {
			maxTime = maxTimeSvc
		}
		tenantServiceMeta, err := bw.Flush(ctx, group.svc)
		if err != nil {
			return nil, err
		}
		tenantServiceMeta.Name = group.svc
		tenantServiceMeta.MinTime = minTimeSvc
		tenantServiceMeta.MaxTime = maxTimeSvc
		bw.meta.TenantServices = append(bw.meta.TenantServices, tenantServiceMeta)

		sWriter, _ := bw.getOrCreateService(group.svc)
		err = createBlockFile(sWriter.path)
		if err != nil {
			return nil, err
		}

		err = appendFileContent(blockFile, filepath.Join(sWriter.path, "block.bin"))
		if err != nil {
			return nil, err
		}
		for i := range tenantServiceMeta.TableOfContents {
			tenantServiceMeta.TableOfContents[i] += offset
		}
		offset += tenantServiceMeta.Size
	}

	meta := bw.meta
	meta.MinTime = minTime
	meta.MaxTime = maxTime
	meta.Size = offset // it already holds the sum of sizes for all tenant services

	return meta, nil
}

func createBlockFile(path string) error {
	file, err := os.Create(filepath.Join(path, "block.bin"))
	if err != nil {
		return err
	}
	defer file.Close()

	for _, sourceFile := range []string{"profiles.parquet", "index.tsdb", "symbols.symdb"} {
		err := appendFileContent(file, filepath.Join(path, sourceFile))
		if err != nil {
			return err
		}
	}

	return nil
}

func appendFileContent(dst *os.File, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	_, err = io.Copy(dst, src)
	return err
}

type profileRowIterator struct {
	profiles    iter.Iterator[parquet.Row]
	blockReader *serviceReader
	closer      io.Closer
	index       phlaredb.IndexReader
	allPostings index.Postings
	err         error

	currentRow       profileRow
	currentSeriesIdx uint32
	chunks           []index.ChunkMeta
}

func newProfileRowIterator(s *serviceReader) (*profileRowIterator, error) {
	k, v := index.AllPostingsKey()
	allPostings, err := s.indexReader.Postings(k, nil, v)
	if err != nil {
		return nil, err
	}
	// todo close once https://github.com/grafana/pyroscope/issues/2172 is done.
	reader := parquet.NewReader(s.profilesReader, schemav1.ProfilesSchema)
	return &profileRowIterator{
		profiles:         phlareparquet.NewBufferedRowReaderIterator(reader, 32),
		blockReader:      s,
		closer:           reader,
		index:            s.indexReader,
		allPostings:      allPostings,
		currentSeriesIdx: math.MaxUint32,
		chunks:           make([]index.ChunkMeta, 1),
	}, nil
}

func (p *profileRowIterator) At() profileRow {
	return p.currentRow
}

func (p *profileRowIterator) Next() bool {
	if !p.profiles.Next() {
		return false
	}
	p.currentRow.serviceReader = p.blockReader
	p.currentRow.row = schemav1.ProfileRow(p.profiles.At())
	seriesIndex := p.currentRow.row.SeriesIndex()
	p.currentRow.timeNanos = p.currentRow.row.TimeNanos()
	// do we have a new series?
	if seriesIndex == p.currentSeriesIdx {
		return true
	}
	p.currentSeriesIdx = seriesIndex
	if !p.allPostings.Next() {
		if err := p.allPostings.Err(); err != nil {
			p.err = err
			return false
		}
		p.err = errors.New("unexpected end of postings")
		return false
	}

	fp, err := p.index.Series(p.allPostings.At(), &p.currentRow.labels, &p.chunks)
	if err != nil {
		p.err = err
		return false
	}
	p.currentRow.fp = model.Fingerprint(fp)
	return true
}

func (p *profileRowIterator) Err() error {
	if p.err != nil {
		return p.err
	}
	return p.profiles.Err()
}

func (p *profileRowIterator) Close() error {
	err := p.profiles.Close()
	if p.closer != nil {
		if err := p.closer.Close(); err != nil {
			return err
		}
	}
	return err
}

type dedupeProfileRowIterator struct {
	iter.Iterator[profileRow]

	prevFP        model.Fingerprint
	prevTimeNanos int64
}

func (it *dedupeProfileRowIterator) Next() bool {
	for {
		if !it.Iterator.Next() {
			return false
		}
		currentProfile := it.Iterator.At()
		if it.prevFP == currentProfile.fp && it.prevTimeNanos == currentProfile.timeNanos {
			// skip duplicate profile
			continue
		}
		it.prevFP = currentProfile.fp
		it.prevTimeNanos = currentProfile.timeNanos
		return true
	}
}

func (w *Worker) createBlockWriter(dest, tenant string, shard, compactionLevel uint32) (*blockWriter, error) {
	meta := &metastorev1.BlockMeta{}
	meta.Id = ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	meta.TenantId = tenant
	meta.Shard = shard
	meta.CompactionLevel = compactionLevel
	return newBlockWriter(meta, dest)
}

func newBlockWriter(meta *metastorev1.BlockMeta, dest string) (*blockWriter, error) {
	blockPath := filepath.Join(dest)

	err := os.RemoveAll(blockPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(blockPath, 0o777); err != nil {
		return nil, err
	}

	return &blockWriter{
		serviceWriters: make(map[string]*serviceWriter),
		path:           blockPath,
		meta:           meta,
		totalProfiles:  0,
	}, nil
}

type serviceWriter struct {
	indexRewriter   *indexRewriter
	symbolsRewriter SymbolsRewriter
	profilesWriter  *profilesWriter
	path            string
}

type blockWriter struct {
	serviceWriters map[string]*serviceWriter

	path          string
	meta          *metastorev1.BlockMeta
	totalProfiles uint64
}

func (bw *blockWriter) getOrCreateService(svc string) (*serviceWriter, error) {
	sw, ok := bw.serviceWriters[svc]
	if !ok {
		path := filepath.Join(bw.path, strconv.Itoa(len(bw.serviceWriters)))
		if err := os.MkdirAll(path, 0o777); err != nil {
			return nil, err
		}
		profileWriter, err := newProfileWriter(path)
		if err != nil {
			return nil, err
		}
		symbolsCompactor := newSymbolsCompactor(path, symdb.FormatV3)
		sw = &serviceWriter{
			indexRewriter:   newIndexRewriter(path),
			symbolsRewriter: symbolsCompactor.Rewriter(path),
			profilesWriter:  profileWriter,
			path:            path,
		}
		bw.serviceWriters[svc] = sw
	}
	return sw, nil
}

func (bw *blockWriter) WriteRow(svc string, r profileRow) error {
	sw, err := bw.getOrCreateService(svc)
	if err != nil {
		return err
	}
	err = sw.indexRewriter.ReWriteRow(r)
	if err != nil {
		return err
	}
	err = sw.symbolsRewriter.ReWriteRow(r)
	if err != nil {
		return err
	}

	if err := sw.profilesWriter.WriteRow(r); err != nil {
		return err
	}
	bw.totalProfiles++
	return nil
}

func (bw *blockWriter) Flush(ctx context.Context, service string) (*metastorev1.TenantService, error) {
	sw, err := bw.getOrCreateService(service)
	if err != nil {
		return nil, err
	}
	offsets, totalSize, err := sw.Close(ctx)
	if err != nil {
		return nil, err
	}
	tenantService := &metastorev1.TenantService{
		TenantId:        bw.meta.TenantId,
		Name:            service,
		TableOfContents: offsets,
		Size:            totalSize,
		ProfileTypes:    nil, // TODO
	}
	return tenantService, nil
}

func (sw *serviceWriter) Close(ctx context.Context) (offsets []uint64, totalSize uint64, err error) {
	if err := sw.profilesWriter.Close(); err != nil {
		return nil, 0, err
	}
	profilesInfo, err := os.Stat(filepath.Join(sw.path, "profiles.parquet"))
	if err != nil {
		return nil, 0, err
	}
	if err := sw.indexRewriter.Close(ctx); err != nil {
		return nil, 0, err
	}
	indexInfo, err := os.Stat(filepath.Join(sw.path, "index.tsdb"))
	if err != nil {
		return nil, 0, err
	}
	if err := sw.symbolsRewriter.Close(); err != nil {
		return nil, 0, err
	}
	symbolsInfo, err := os.Stat(filepath.Join(sw.path, "symbols.symdb"))
	if err != nil {
		return nil, 0, err
	}
	offsets = []uint64{0, uint64(profilesInfo.Size()), uint64(profilesInfo.Size() + indexInfo.Size())}
	totalSize = uint64(indexInfo.Size()) + uint64(symbolsInfo.Size()) + uint64(profilesInfo.Size())
	return offsets, totalSize, nil
}

type profilesWriter struct {
	*parquet.GenericWriter[*schemav1.Profile]
	file *os.File

	buf []parquet.Row
}

func newProfileWriter(path string) (*profilesWriter, error) {
	profilePath := filepath.Join(path, (&schemav1.ProfilePersister{}).Name()+block.ParquetSuffix)
	profileFile, err := os.OpenFile(profilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	return &profilesWriter{
		GenericWriter: newParquetProfileWriter(profileFile, parquet.MaxRowsPerRowGroup(100_000)),
		file:          profileFile,
		buf:           make([]parquet.Row, 1),
	}, nil
}

func newParquetProfileWriter(writer io.Writer, options ...parquet.WriterOption) *parquet.GenericWriter[*schemav1.Profile] {
	options = append(options, parquet.PageBufferSize(32*1024))
	options = append(options, parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision))
	options = append(options, schemav1.ProfilesSchema)
	return parquet.NewGenericWriter[*schemav1.Profile](
		writer, options...,
	)
}

func (p *profilesWriter) WriteRow(r profileRow) error {
	p.buf[0] = parquet.Row(r.row)
	_, err := p.GenericWriter.WriteRows(p.buf)
	if err != nil {
		return err
	}

	return nil
}

func (p *profilesWriter) Close() error {
	err := p.GenericWriter.Close()
	if err != nil {
		return err
	}
	return p.file.Close()
}

func newIndexRewriter(path string) *indexRewriter {
	return &indexRewriter{
		symbols: make(map[string]struct{}),
		path:    path,
	}
}

type indexRewriter struct {
	series []struct {
		labels phlaremodel.Labels
		fp     model.Fingerprint
	}
	symbols map[string]struct{}
	chunks  []index.ChunkMeta // one chunk per series

	previousFp model.Fingerprint

	path string
}

func (idxRw *indexRewriter) ReWriteRow(r profileRow) error {
	if idxRw.previousFp != r.fp || len(idxRw.series) == 0 {
		series := r.labels.Clone()
		for _, l := range series {
			idxRw.symbols[l.Name] = struct{}{}
			idxRw.symbols[l.Value] = struct{}{}
		}
		idxRw.series = append(idxRw.series, struct {
			labels phlaremodel.Labels
			fp     model.Fingerprint
		}{
			labels: series,
			fp:     r.fp,
		})
		idxRw.chunks = append(idxRw.chunks, index.ChunkMeta{
			MinTime:     r.timeNanos,
			MaxTime:     r.timeNanos,
			SeriesIndex: uint32(len(idxRw.series) - 1),
		})
		idxRw.previousFp = r.fp
	}
	idxRw.chunks[len(idxRw.chunks)-1].MaxTime = r.timeNanos
	r.row.SetSeriesIndex(idxRw.chunks[len(idxRw.chunks)-1].SeriesIndex)
	return nil
}

func (idxRw *indexRewriter) NumSeries() uint64 {
	return uint64(len(idxRw.series))
}

// Close writes the index to given folder.
func (idxRw *indexRewriter) Close(ctx context.Context) error {
	indexw, err := index.NewWriter(ctx, filepath.Join(idxRw.path, block.IndexFilename), 1<<18)
	if err != nil {
		return err
	}

	// Sort symbols
	symbols := make([]string, 0, len(idxRw.symbols))
	for s := range idxRw.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Add symbols
	for _, symbol := range symbols {
		if err := indexw.AddSymbol(symbol); err != nil {
			return err
		}
	}

	// Add Series
	for i, series := range idxRw.series {
		if err := indexw.AddSeries(storage.SeriesRef(i), series.labels, series.fp, idxRw.chunks[i]); err != nil {
			return err
		}
	}

	return indexw.Close()
}
