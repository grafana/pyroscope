package convert

// These functions are kept separately as profile.pb.go is a generated file

import "strings"

func (profile *Profile) Get(sampleType string, cb func(name []byte, val int)) error {
	valueIndex := 0
	if sampleType != "" {
		for i, v := range profile.SampleType {
			if profile.StringTable[v.Type] == sampleType {
				valueIndex = i
				break
			}
		}
	}

	locations := make(map[uint64]*Location, len(profile.Location))
	for _, l := range profile.Location {
		locations[l.Id] = l
	}

	functions := make(map[uint64]*Function, len(profile.Function))
	for _, f := range profile.Function {
		functions[f.Id] = f
	}

	for _, s := range profile.Sample {
		stack := []string{}
		for _, lID := range s.LocationId {
			l := locations[lID]
			fID := l.Line[0].FunctionId
			f := functions[fID]
			stack = append([]string{profile.StringTable[f.Name]}, stack...)
		}
		name := strings.Join(stack, ";")
		cb([]byte(name), int(s.Value[valueIndex]))
	}
	return nil
}
