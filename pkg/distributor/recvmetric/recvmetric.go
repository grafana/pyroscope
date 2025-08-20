package recvmetric

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
)

type Stage int

const (
	// StageReceived is the earliest stage
	// Recorded as soon as we begin processing a profile, before any rate-limit/sampling checks
	StageReceived Stage = iota
	// StageSampled is recorded after the profile is not discarded by rate-limit/sampling checks
	StageSampled
	// StageNormalized is recorded after the profile is validated and normalized.
	// If the profile passed rate-limit/sampling checks but failed validation checks, then
	// the size of StageSampled is used
	StageNormalized
	totalStages
)

func (s Stage) String() string {
	switch s {
	case StageSampled:
		return "sampled"
	case StageReceived:
		return "received"
	case StageNormalized:
		return "normalized"
	default:
		return fmt.Sprintf("unknown stage %d", int(s))
	}
}

func (s *Stage) Set(str string) error {
	switch str {
	case "sampled":
		*s = StageSampled
		return nil
	case "received":
		*s = StageReceived
		return nil
	case "normalized":
		*s = StageNormalized
		return nil
	default:
		return fmt.Errorf("unexpected Stage value: %s. valid values are: %s %s %s", str,
			StageSampled.String(), StageReceived.String(), StageNormalized.String())
	}
}

func (s *Stage) UnmarshalYAML(value *yaml.Node) error {
	m := ""
	err := value.DecodeWithOptions(&m, yaml.DecodeOptions{
		KnownFields: true,
	})
	if err != nil {
		return fmt.Errorf("malformed Stage: %w %+v", err, value)
	}
	return s.Set(m)
}

func (s Stage) MarshalYAML() (interface{}, error) {
	return s.String(), nil
}

const metricName = "distributor_received_decompressed_bytes_total"
const NamespacedMetricName = "pyroscope_" + metricName

type Metric struct {
	metric *prometheus.HistogramVec
	//	todo move discarded and usage group metrics to this package
}

func New(reg prometheus.Registerer) *Metric {
	const (
		minBytes     = 10 * 1024
		maxBytes     = 15 * 1024 * 1024
		bucketsCount = 30
	)
	m := &Metric{
		metric: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      metricName,
				Help:      "The total number of decompressed bytes per profile received by the distributor at different processing stages.",
				Buckets:   prometheus.ExponentialBucketsRange(minBytes, maxBytes, bucketsCount),
			},
			[]string{
				"tenant",
				"stage",
				"tenant_stage", // bool: "true" if this stage matches the tenant's configured stage
			},
		),
	}
	if reg != nil {
		reg.MustRegister(m.metric)
	}
	return m
}

func (r *Metric) NewRequest(tenant string, tenantStage Stage, receivedSize uint64) *Request {
	res := &Request{
		metric:      r.metric,
		tenant:      tenant,
		tenantStage: tenantStage,
	}
	res.set[StageReceived] = true
	res.sizes[StageReceived] = receivedSize
	return res
}

type Request struct {
	metric      *prometheus.HistogramVec
	tenant      string
	tenantStage Stage
	sizes       [totalStages]uint64
	set         [totalStages]bool
}

func (m *Request) Record(stage Stage, size uint64) {
	if stage == StageReceived {
		panic("programming error. StageReceived is recorded at Request initialization")
	}
	m.set[stage] = true
	m.sizes[stage] = size
}

func (m *Request) Observe() {
	m.observe(StageReceived)
	if !m.set[StageSampled] {
		return
	}
	m.observe(StageSampled)
	if !m.set[StageNormalized] {
		// Invalid profile. Normalization happens after validation for valid profiles.
		// If we did not perform normalization - use previous stage value
		m.sizes[StageNormalized] = m.sizes[StageSampled]
		m.set[StageNormalized] = true
	}
	m.observe(StageNormalized)

}

func (m *Request) observe(stage Stage) {
	sz := m.sizes[stage]
	if !m.set[stage] {
		panic(fmt.Sprintf("Received metric stage %d not set", stage))
	}
	isTenantStage := "false"
	if stage == m.tenantStage {
		isTenantStage = "true"
	}
	m.metric.WithLabelValues(m.tenant, stage.String(), isTenantStage).Observe(float64(sz))
}
