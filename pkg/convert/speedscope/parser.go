package speedscope

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

// RawProfile implements ingestion.RawProfile for Speedscope format
type RawProfile struct {
	RawData []byte
}

// Parse parses a profile
func (p *RawProfile) Parse(ctx context.Context, putter storage.Putter, _ storage.MetricsExporter, md ingestion.Metadata) error {
	profiles, err := parseAll(p.RawData, md)
	if err != nil {
		return err
	}

	for _, putInput := range profiles {
		err = putter.Put(ctx, putInput)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseAll(rawData []byte, md ingestion.Metadata) ([]*storage.PutInput, error) {
	file := speedscopeFile{}
	err := json.Unmarshal(rawData, &file)
	if err != nil {
		return nil, err
	}
	if file.Schema != schema {
		return nil, fmt.Errorf("Unknown schema: %s", file.Schema)
	}

	results := make([]*storage.PutInput, 0, len(file.Profiles))
	// Not a pointer, we _want_ to copy on call
	input := storage.PutInput{
		StartTime:  md.StartTime,
		EndTime:    md.EndTime,
		SpyName:    md.SpyName,
		SampleRate: md.SampleRate,
		Key:        md.Key,
	}

	for _, prof := range file.Profiles {
		putInput, err := parseOne(&prof, input, file.Shared.Frames, len(file.Profiles) > 1)
		if err != nil {
			return nil, err
		}
		results = append(results, putInput)
	}
	return results, nil
}

func parseOne(prof *profile, putInput storage.PutInput, frames []frame, multi bool) (*storage.PutInput, error) {
	// Fixup some metadata
	putInput.Units = chooseUnit(prof.Unit)
	putInput.AggregationType = metadata.SumAggregationType
	if multi {
		putInput.Key = chooseKey(putInput.Key, prof.Unit)
	}
	// Don't override sampleRate. Sometimes units corresponds to that, but not necessarily.

	var err error
	tr := tree.New()
	switch prof.Type {
	case profileEvented:
		err = parseEvented(tr, prof, frames)
	case profileSampled:
		err = parseSampled(tr, prof, frames)
	default:
		return nil, fmt.Errorf("Profile type %s not supported", prof.Type)
	}
	if err != nil {
		return nil, err
	}

	putInput.Val = tr
	return &putInput, nil
}

func chooseUnit(unit string) metadata.Units {
	switch unit {
	case unitBytes:
		return metadata.BytesUnits
	default:
		return metadata.SamplesUnits
	}
}

func chooseKey(orig *segment.Key, unit string) *segment.Key {
	// This means we'll have duplicate keys if multiple profiles have the same units. Probably ok.
	name := fmt.Sprintf("%s.%s", orig.AppName(), unit)
	result := orig.Clone()
	result.Add("__name__", name)
	return result
}

func parseEvented(tr *tree.Tree, prof *profile, frames []frame) error {
	last := prof.StartValue
	indexStack := []int{}
	nameStack := []string{}

	for _, ev := range prof.Events {
		if ev.At < last {
			return fmt.Errorf("Events out of order, %f < %f", ev.At, last)
		}
		fid := int(ev.Frame)
		if fid < 0 || fid >= len(frames) {
			return fmt.Errorf("Invalid frame %d", fid)
		}

		if ev.Type == eventClose {
			if len(indexStack) == 0 {
				return fmt.Errorf("No stack to close at %f", ev.At)
			}
			lastIdx := len(indexStack) - 1
			if indexStack[lastIdx] != fid {
				return fmt.Errorf("Closing non-open frame %d", fid)
			}

			// Close this frame
			tr.InsertStackString(nameStack, uint64(ev.At-last))
			indexStack = indexStack[:lastIdx]
			nameStack = nameStack[:lastIdx]

		} else if ev.Type == eventOpen {
			// Add any time up til now
			if len(nameStack) > 0 {
				tr.InsertStackString(nameStack, uint64(ev.At-last))
			}

			// Open the frame
			indexStack = append(indexStack, fid)
			nameStack = append(nameStack, frames[fid].Name)
		} else {
			return fmt.Errorf("Unknown event type %s", ev.Type)
		}

		last = ev.At
	}

	return nil
}

func parseSampled(tr *tree.Tree, prof *profile, frames []frame) error {
	if len(prof.Samples) != len(prof.Weights) {
		return fmt.Errorf("Unequal lengths of samples and weights: %d != %d", len(prof.Samples), len(prof.Weights))
	}

	stack := []string{}
	for i, samp := range prof.Samples {
		weight := prof.Weights[i]
		if weight < 0 {
			return fmt.Errorf("Negative weight %f", weight)
		}

		for _, frameId := range samp {
			fid := int(frameId)
			if fid < 0 || fid > len(frames) {
				return fmt.Errorf("Invalid frame %d", fid)
			}
			stack = append(stack, frames[fid].Name)
		}
		tr.InsertStackString(stack, uint64(weight))

		stack = stack[:0] // clear, but retain memory
	}
	return nil
}

// Bytes returns the raw bytes of the profile
func (p *RawProfile) Bytes() ([]byte, error) {
	return p.RawData, nil
}

// ContentType returns the HTTP ContentType of the profile
func (p *RawProfile) ContentType() string {
	return "application/json"
}
