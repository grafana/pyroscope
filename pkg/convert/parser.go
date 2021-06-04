package convert

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strconv"

	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"google.golang.org/protobuf/proto"
)

// format is a Serialized trie (see transporttrie.Serialize implementation)
func ParseTrie(r io.Reader, cb func(name []byte, val int)) error {
	trie, err := transporttrie.Deserialize(r)
	if err != nil {
		return err
	}
	trie.Iterate(func(name []byte, val uint64) {
		cb(name, int(val))
	})
	return nil
}

// format is pprof. See https://github.com/google/pprof/blob/master/proto/profile.proto
func ParsePprof(r io.Reader) (*Profile, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	profile := &Profile{}
	if err := proto.Unmarshal(b, profile); err != nil {
		return nil, err
	}
	return profile, nil
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

		scannerLine := scanner.Bytes()
		line := make([]byte, len(scannerLine))
		copy(line, scannerLine)

		index := bytes.LastIndexByte(line, byte(' '))
		if index == -1 {
			continue
		}
		stacktrace := line[:index]
		count := line[index+1:]

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
