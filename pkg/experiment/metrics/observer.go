package metrics

import (
	"fmt"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
)

type MetricsExporterSampleObserver struct {
	tenant            string
	recorder          *Recorder
	recordingTime     int64
	pyroscopeInstance string
	logger            log.Logger
}

func NewMetricsExporterSampleObserver(meta *metastorev1.BlockMeta, logger log.Logger) *MetricsExporterSampleObserver {
	recordingTime := int64(ulid.MustParse(meta.Id).Time())
	pyroscopeInstance := pyroscopeInstanceHash(meta.Shard, meta.CreatedBy)
	return &MetricsExporterSampleObserver{
		recordingTime:     recordingTime,
		pyroscopeInstance: pyroscopeInstance,
		logger:            logger,
	}
}

func pyroscopeInstanceHash(shard uint32, createdBy int32) string {
	buf := make([]byte, 0, 8)
	buf = append(buf, byte(shard>>24), byte(shard>>16), byte(shard>>8), byte(shard))
	buf = append(buf, byte(createdBy>>24), byte(createdBy>>16), byte(createdBy>>8), byte(createdBy))
	return fmt.Sprintf("%x", xxhash.Sum64(buf))
}

func (o *MetricsExporterSampleObserver) Init(tenant string) {
	o.tenant = tenant
	o.recorder = NewRecorder(recordingRulesFromTenant(tenant), o.recordingTime, o.pyroscopeInstance)
}

func (o *MetricsExporterSampleObserver) Observe(row block.ProfileEntry) {
	o.recorder.RecordRow(row.Fingerprint, row.Labels, row.Row.TotalValue())
}

func (o *MetricsExporterSampleObserver) Flush() error {
	rec := o.recorder
	o.recorder = nil
	go func(tenant string, recorder *Recorder) {
		exporter, err := NewExporter(tenant, recorder.Recordings)
		if err != nil {
			level.Error(o.logger).Log("msg", "error creating metrics exporter", "err", err)
			return
		}

		if err = exporter.Send(); err != nil {
			level.Error(o.logger).Log("msg", "error sending recording metrics", "err", err)
		}
	}(o.tenant, rec)
	return nil
}
