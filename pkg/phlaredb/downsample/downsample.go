package downsample

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dolthub/swiss"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
)

type interval struct {
	durationSeconds int64
	shortName       string
}

type aggregationType struct {
	fn   func(a, b int64) int64
	name string
}

type state struct {
	currentRow          parquet.Row
	currentTime         int64
	currentFp           model.Fingerprint
	totalValue          int64
	profileCount        int64
	stackTraceIds       []uint64
	values              []int64
	stackTraceIdToIndex *swiss.Map[uint64, int]
}

type downsampleConfig struct {
	interval    interval
	aggregation aggregationType
}

var (
	intervals = []interval{
		{
			durationSeconds: 5 * 60,
			shortName:       "5m",
		},
		{
			durationSeconds: 60 * 60,
			shortName:       "1h",
		},
	}
	aggregations = []aggregationType{
		{
			name: "sum",
			fn: func(a, b int64) int64 {
				return a + b
			},
		},
	}
	configs               = initConfigs()
	inputSamplesHistogram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pyroscope_downsampler_input_profile_samples",
			Help:    "The number of samples per profile before downsampling",
			Buckets: prometheus.ExponentialBuckets(32, 2, 15),
		})
	outputSamplesHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pyroscope_downsampler_output_profile_samples",
			Help:    "The number of samples per profile after downsampling",
			Buckets: prometheus.ExponentialBuckets(32, 2, 15),
		}, []string{"interval"})
)

func initConfigs() []downsampleConfig {
	configs := make([]downsampleConfig, 0)
	for _, i := range intervals {
		for _, a := range aggregations {
			configs = append(configs, downsampleConfig{
				interval:    i,
				aggregation: a,
			})
		}
	}
	return configs
}

type profilesWriter struct {
	*parquet.GenericWriter[*schemav1.Profile]
	file *os.File

	buf []parquet.Row
}

func (p *profilesWriter) WriteRow(r parquet.Row) error {
	p.buf[0] = r
	_, err := p.GenericWriter.WriteRows(p.buf)
	if err != nil {
		return err
	}

	return nil
}

func newProfilesWriter(path string, i interval, aggregation string) (*profilesWriter, error) {
	profilePath := filepath.Join(path, fmt.Sprintf("profiles_%s_%s", i.shortName, aggregation)+block.ParquetSuffix)
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
	options = append(options, parquet.PageBufferSize(3*1024*1024))
	options = append(options, parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision))
	options = append(options, schemav1.DownsampledProfilesSchema)
	return parquet.NewGenericWriter[*schemav1.Profile](
		writer, options...,
	)
}

type Downsampler struct {
	path           string
	profileWriters []*profilesWriter
	states         []*state
	logger         log.Logger
}

func NewDownsampler(path string, logger log.Logger) (*Downsampler, error) {
	writers := make([]*profilesWriter, 0)
	states := make([]*state, 0)
	for _, c := range configs {
		writer, err := newProfilesWriter(path, c.interval, c.aggregation.name)
		if err != nil {
			return nil, err
		}
		writers = append(writers, writer)
		states = append(states, &state{})
	}

	return &Downsampler{
		path:           path,
		profileWriters: writers,
		states:         states,
		logger:         logger,
	}, nil
}

func (d *Downsampler) flush(s *state, w *profilesWriter, c downsampleConfig) error {
	level.Debug(d.logger).Log(
		"msg", "flushing downsampled profile",
		"interval", c.interval.shortName,
		"aggregation", c.aggregation.name,
		"sourceProfileCount", s.profileCount,
		"sampleCount", len(s.values))
	outputSamplesHistogram.WithLabelValues(c.interval.shortName).Observe(float64(len(s.values)))
	var (
		col    = len(s.currentRow) - 1
		newCol = func() int {
			col++
			return col
		}
	)
	s.currentRow = append(s.currentRow, parquet.Int64Value(s.totalValue).Level(0, 0, newCol()))

	newCol()
	repetition := -1
	for _, stacktraceId := range s.stackTraceIds {
		if repetition < 1 {
			repetition++
		}
		s.currentRow = append(s.currentRow, parquet.Int64Value(int64(stacktraceId)).Level(repetition, 1, col))
	}
	newCol()
	repetition = -1
	for _, value := range s.values {
		if repetition < 1 {
			repetition++
		}
		s.currentRow = append(s.currentRow, parquet.Int64Value(value).Level(repetition, 1, col))
	}

	s.currentRow = append(s.currentRow, parquet.Int64Value(s.currentTime*1000*1000*1000).Level(0, 0, newCol()))

	err := w.WriteRow(s.currentRow)
	if err != nil {
		return err
	}
	return nil
}

func (d *Downsampler) AddRow(row schemav1.ProfileRow, fp model.Fingerprint) error {
	rowTimeSeconds := row.TimeNanos() / 1000 / 1000 / 1000
	sourceSampleCount := 0
	for i, c := range configs {
		s := d.states[i]
		aggregationTime := rowTimeSeconds / c.interval.durationSeconds * c.interval.durationSeconds
		if len(d.states[i].currentRow) == 0 {
			d.initStateFromRow(s, row, aggregationTime, fp)
		}
		if s.currentTime != aggregationTime || s.currentFp != fp {
			err := d.flush(s, d.profileWriters[i], c)
			if err != nil {
				return err
			}
			d.initStateFromRow(s, row, aggregationTime, fp)
		}
		s.profileCount++
		row.ForStacktraceIdsAndValues(func(stacktraceIds []parquet.Value, values []parquet.Value) {
			for i := 0; i < len(stacktraceIds); i++ {
				stacktraceId := stacktraceIds[i].Uint64()
				value := values[i].Int64()
				index, ok := s.stackTraceIdToIndex.Get(stacktraceId)
				if ok {
					s.values[index] = c.aggregation.fn(s.values[index], value)
				} else {
					s.stackTraceIds = append(s.stackTraceIds, stacktraceId)
					s.values = append(s.values, value)
					s.stackTraceIdToIndex.Put(stacktraceId, len(s.values)-1)
				}
				s.totalValue = c.aggregation.fn(s.totalValue, value)
			}
			sourceSampleCount = len(values)
		})
	}
	inputSamplesHistogram.Observe(float64(sourceSampleCount))
	return nil
}

func (d *Downsampler) Close() error {
	for i, c := range configs {
		if len(d.states[i].currentRow) > 0 {
			err := d.flush(d.states[i], d.profileWriters[i], c)
			if err != nil {
				return err
			}
		}
		err := d.profileWriters[i].Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Downsampler) initStateFromRow(s *state, row schemav1.ProfileRow, aggregationTime int64, fp model.Fingerprint) {
	s.currentTime = aggregationTime
	s.currentFp = fp
	s.totalValue = 0
	s.profileCount = 0
	if s.values == nil {
		s.values = make([]int64, 0, len(row))
	} else {
		s.values = s.values[:0]
	}
	if s.stackTraceIds == nil {
		s.stackTraceIds = make([]uint64, 0, len(row))
	} else {
		s.stackTraceIds = s.stackTraceIds[:0]
	}
	if s.stackTraceIdToIndex == nil {
		s.stackTraceIdToIndex = swiss.NewMap[uint64, int](uint32(len(row)))
	} else {
		s.stackTraceIdToIndex.Clear()
	}
	var (
		col    = -1
		newCol = func() int {
			col++
			return col
		}
	)
	if s.currentRow == nil {
		s.currentRow = make(parquet.Row, 0, len(row)) // we might need to make this bigger
	} else {
		s.currentRow = s.currentRow[:0]
	}
	s.currentRow = append(s.currentRow, parquet.Int32Value(int32(row.SeriesIndex())).Level(0, 0, newCol()))
	s.currentRow = append(s.currentRow, parquet.Int64Value(int64(row.StacktracePartitionID())).Level(0, 0, newCol()))
}
