package convert

import (
	"fmt"
	"io"
	"strings"

	"github.com/pyroscope-io/jfr-parser/parser"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

func ParseJFR(r io.Reader, cb func(name []byte, val int), pi *storage.PutInput) error {
	// Only execution sample events are supported for now.
	chunks, err := parser.Parse(r)
	if err != nil {
		return fmt.Errorf("unable to parse JFR format: %w", err)
	}
	event := "cpu"
	for _, c := range chunks {
		// TODO(abeaumont): support multiple chunks
		for _, e := range c.Events {
			switch e.(type) {
			case *parser.ExecutionSample:
				es := e.(*parser.ExecutionSample)
				if es.StackTrace != nil {
					frames := make([]string, 0, len(es.StackTrace.Frames))
					for i := len(es.StackTrace.Frames) - 1; i >= 0; i-- {
						f := es.StackTrace.Frames[i]
						// TODO(abeaumont): Add support for line numbers.
						if f.Method != nil && f.Method.Type != nil && f.Method.Type.Name != nil && f.Method.Name != nil {
							frames = append(frames, f.Method.Type.Name.String+"."+f.Method.Name.String)
						}
					}
					if len(frames) > 0 {
						cb([]byte(strings.Join(frames, ";")), 1)
					}
				}
			case *parser.ActiveSetting:
				as := e.(*parser.ActiveSetting)
				if as.Name == "event" {
					event = as.Value
				}
			}
		}
		labels := pi.Key.Labels()
		labels["__name__"] += "." + event
		pi.Key = segment.NewKey(labels)
	}
	return nil
}
