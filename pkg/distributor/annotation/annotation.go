package annotation

const (
	ProfileAnnotationKeyThrottled = "pyroscope.ingest.throttled"
	ProfileAnnotationKeySampled   = "pyroscope.ingest.sampled"
)

type ProfileAnnotation struct {
	Body interface{} `json:"body"`
}
