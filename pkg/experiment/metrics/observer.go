package metrics

import (
	"fmt"

	"github.com/cespare/xxhash/v2"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
)

type MetricsExporterSampleObserver struct {
	tenant   string
	recorder *Recorder
}

func NewMetricsExporterSampleObserver(tenant string, meta *metastorev1.BlockMeta) *MetricsExporterSampleObserver {
	recordingTime := int64(ulid.MustParse(meta.Id).Time())
	rules := recordingRulesFromTenant(tenant)
	pyroscopeInstance := pyroscopeInstanceHash(meta.Shard, meta.CreatedBy)
	return &MetricsExporterSampleObserver{
		tenant:   tenant,
		recorder: NewRecorder(rules, recordingTime, pyroscopeInstance),
	}
}

func pyroscopeInstanceHash(shard uint32, createdBy int32) string {
	buf := make([]byte, 0, 8)
	buf = append(buf, byte(shard>>24), byte(shard>>16), byte(shard>>8), byte(shard))
	buf = append(buf, byte(createdBy>>24), byte(createdBy>>16), byte(createdBy>>8), byte(createdBy))
	return fmt.Sprintf("%x", xxhash.Sum64(buf))
}

func (o *MetricsExporterSampleObserver) Observe(row block.ProfileEntry) {
	o.recorder.RecordRow(row.Fingerprint, row.Labels, row.Row.TotalValue())
}

func (o *MetricsExporterSampleObserver) Flush() error {
	go func() {
		NewExporter(o.tenant, o.recorder.Recordings).Send() // TODO log error
	}()
	return nil
}
