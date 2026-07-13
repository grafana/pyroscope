package annotation

const (
	ProfileAnnotationKeyThrottled = "pyroscope.ingest.throttled"
	ProfileAnnotationKeySampled   = "pyroscope.ingest.sampled"
	ProfileAnnotationKeyStripped  = "pyroscope.ingest.sampled.stripped"
)

type ProfileAnnotation struct {
	Body interface{} `json:"body"`
}
