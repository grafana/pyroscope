package model

import "github.com/pyroscope-io/pyroscope/pkg/storage/metadata"

type Application struct {
	// TODO: Fully qualified or just name? {__name__}.{profile_type} vs __name__
	//   app <- app name
	//   app.cpu <- app fully qualified name
	//   app.cpu{key=value} <- series name
	Name string

	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType
}
