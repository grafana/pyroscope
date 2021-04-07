package convert

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"google.golang.org/protobuf/proto"
)

// format is a Serialized trie (see transporttrie.Serialize implementation)
func ParseTrie(r io.Reader, cb func(name []byte, val int)) error {
	t, _ := transporttrie.Deserialize(r)
	t.Iterate(func(name []byte, val uint64) {
		cb(name, int(val))
	})
	return nil
}

// format is pprof. See https://github.com/google/pprof/blob/master/proto/profile.proto
func ParsePprof(r io.Reader, cb func(name []byte, val int)) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	profile := &Profile{}
	if err := proto.Unmarshal(b, profile); err != nil {
		return err
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
		cb([]byte(name), int(s.Value[0]))
	}
	return nil
}

// format:
// stack-trace-foo 1
// stack-trace-bar 2
func ParseGroups(r io.Reader, cb func(name []byte, val int)) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}

		line := scanner.Bytes()
		line2 := make([]byte, len(line))
		copy(line2, line)

		index := bytes.LastIndexByte(line2, byte(' '))
		if index == -1 {
			continue
		}
		stacktrace := line2[:index]
		count := line2[index+1:]

		i, err := strconv.Atoi(string(count))
		if err != nil {
			return err
		}
		cb(stacktrace, i)
	}
	return nil
}

// format:
// stack-trace-foo
// stack-trace-bar
// stack-trace-bar
func ParseIndividualLines(r io.Reader, cb func(name []byte, val int)) error {
	groups := make(map[string]int)
	scanner := bufio.NewScanner(r)
	// scanner.Buffer(make([]byte, bufio.MaxScanTokenSize*100), bufio.MaxScanTokenSize*100)
	// scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		key := scanner.Text()
		if _, ok := groups[key]; !ok {
			groups[key] = 0
		}
		groups[key]++
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	for k, v := range groups {
		if k != "" {
			cb([]byte(k), v)
		}
	}

	return nil
}
