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
	ActiveProfileIndex int
	Exporter           string
}

type shared struct {
	Frames []frame
}

type frame struct {
	Name string
	File string
	Line int
	Col  int
}

type profile struct {
	Type       string
	Name       string
	Unit       string
	StartValue int64
	EndValue   int64

	// Evented profile
	Events []event

	// Sample profile
	Samples []sample
	Weights []int64
}

type event struct {
	Type  string
	At    int64
	Frame int
}

// Indexes into Frames
type sample []int
