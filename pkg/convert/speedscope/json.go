package speedscope

// Description of Speedscope JSON
// See spec: https://github.com/jlfwong/speedscope/blob/main/src/lib/file-format-spec.ts

const (
	schema = "https://www.speedscope.app/file-format-schema.json"

	profileEvented = "evented"
	profileSampled = "sampled"

	unitNone         = "none"
	unitNanoseconds  = "nanoseconds"
	unitMicroseconds = "microseconds"
	unitMilliseconds = "milliseconds"
	unitSeconds      = "seconds"
	unitBytes        = "bytes"

	eventOpen  = "O"
	eventClose = "C"
)

type speedscopeFile struct {
	Schema             string `json:"$schema"`
	Shared             shared
	Profiles           []profile
	Name               string
	ActiveProfileIndex float64
	Exporter           string
}

type shared struct {
	Frames []frame
}

type frame struct {
	Name string
	File string
	Line float64
	Col  float64
}

type profile struct {
	Type       string
	Name       string
	Unit       string
	StartValue float64
	EndValue   float64

	// Evented profile
	Events []event

	// Sample profile
	Samples []sample
	Weights []float64
}

type event struct {
	Type  string
	At    float64
	Frame float64
}

// Indexes into Frames
type sample []float64
