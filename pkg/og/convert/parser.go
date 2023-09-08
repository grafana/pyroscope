package convert

import (
	"bufio"
	"bytes"
	"io"
	"strconv"

	"github.com/grafana/pyroscope/pkg/og/storage/tree"
)

func ParseTreeNoDict(r io.Reader, cb func(name []byte, val int)) error {
	t, err := tree.DeserializeNoDict(r)
	if err != nil {
		return err
	}
	t.Iterate(func(name []byte, val uint64) {
		if len(name) > 2 && val != 0 {
			cb(name[2:], int(val))
		}
	})
	return nil
}

var gzipMagicBytes = []byte{0x1f, 0x8b}

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
