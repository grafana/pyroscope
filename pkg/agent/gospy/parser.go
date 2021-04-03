package gospy

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/util/strarr"
)

type ParserState int

const (
	goroutineParserState     ParserState = iota
	methodParserState        ParserState = iota
	fileParserState          ParserState = iota
	skipGoroutineParserState ParserState = iota
)

var allowedStates = []string{
	"running",
	"runnable",
}

var m map[string]bool

func init() {
	m = make(map[string]bool)
}

func Parse(r io.Reader, cb func([]byte, uint64, error)) {
	scanner := bufio.NewScanner(r)
	state := goroutineParserState

	goroutineState := ""

	skipLines := 0

	// var stackTrace strings.Builder
	arr := []string{}

	for scanner.Scan() {
		if skipLines > 0 {
			skipLines--
			continue
		}
		line := scanner.Bytes()
		// log.Println(string(line))
		switch state {
		case methodParserState:
			if len(line) == 0 {
				// stackTraceBytes := []byte(stackTrace.String())
				stackTraceBytes := []byte(strings.Join(arr, ";"))
				m[goroutineState] = true
				if !bytes.HasSuffix(stackTraceBytes, []byte("pprof.writeGoroutineStacks")) {
					log.Println("st", goroutineState, string(stackTraceBytes))
					cb(stackTraceBytes[:len(stackTraceBytes)-1], 1, nil)
				}
				// stackTrace.Reset()
				arr = []string{}
				state = goroutineParserState
				continue
			}
			state = fileParserState
			if bytes.HasPrefix(line, []byte("created by")) {
				// skipLines = 2
				arr = append([]string{string(line[11:])}, arr...)
				// stackTrace.Write(line[11:i])
			} else {
				i := bytes.LastIndex(line, []byte("("))
				arr = append([]string{string(line[:i])}, arr...)
				// stackTrace.Write(line[:i])
				// stackTrace.Write([]byte(";"))
			}
		case fileParserState:
			// i := bytes.LastIndex(line, []byte(" "))
			// fileName = string(line[1:i])
			state = methodParserState
		case skipGoroutineParserState:
			if bytes.HasPrefix(line, []byte("created by")) {
				skipLines = 2
				state = goroutineParserState
			}
		case goroutineParserState:
			l := bytes.Index(line, []byte("["))
			r := bytes.Index(line, []byte("]"))
			if l > -1 && r > -1 {
				goroutineState = string(line[l+1 : r])
				// log.Printf("%q", goroutineState)
				if !strarr.Contains(allowedStates, goroutineState) {
					state = skipGoroutineParserState
				} else {
					state = methodParserState
				}
			}
		}
	}

	// if rand.Int31n(100) < 1 {
	// 	log.Println("---")
	// 	for k, _ := range m {
	// 		log.Println("k:", k)
	// 	}
	// }
}
