package perf

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

var reEventStart = regexp.MustCompile("^(\\S.+?)\\s+(\\d+)/*(\\d+)*\\s+\\S.+")
var errEventStartRegexMismatch = fmt.Errorf("reEventStart mismatch")
var reStackFrame = regexp.MustCompile("^\\s*(\\w+)\\s*(.+) \\((\\S*)\\)")
var errStackFrameRegexMismatch = fmt.Errorf("reStackFrame mismatch")
var sep = []byte{'\n'}

type ScriptParser struct {
	lines     [][]byte
	lineIndex int
}

func NewScriptParser(buf []byte) *ScriptParser {
	return &ScriptParser{
		lines:     bytes.Split(buf, sep),
		lineIndex: 0,
	}
}
func (p *ScriptParser) nextLine() ([]byte, error) {
	if p.lineIndex < len(p.lines) {
		ret := p.lines[p.lineIndex]
		p.lineIndex++
		return ret, nil
	}
	return nil, io.EOF
}

func (p *ScriptParser) ParseEvents() ([][][]byte, error) {
	stacks := make([][][]byte, 0, 256)
	for {
		stack, err := p.ParseEvent()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		stacks = append(stacks, stack)
	}
	return stacks, nil
}

func (p *ScriptParser) ParseEvent() ([][]byte, error) {
	line, err := p.nextLine()
	if err != nil {
		return nil, err
	}
	stack := make([][]byte, 0, 16)
	comm, _, _, err := parseEventStart(line)
	if err != nil {
		if len(line) == 0 && p.lineIndex >= len(p.lines) {
			return nil, io.EOF
		}
		return nil, err
	}
	var sym []byte
	for {
		line, err = p.nextLine()
		if err != nil {
			return nil, err
		}
		if parseEventEnd(line) {
			break
		}
		_, sym, _, err = parseStackFrame(line)
		if err != nil {
			return nil, err
		}
		stack = append(stack, sym)
	}
	stack = append(stack, comm)
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack, nil
}

func IsPerfScript(buf []byte) bool {
	ls := bytes.SplitN(buf, sep, 2)
	if len(ls) < 2 {
		return false
	}
	l := ls[0]
	_, _, _, err := parseEventStart(l)
	return err == nil
}

func parseEventStart(line []byte) ([]byte, int, int, error) {
	res := reEventStart.FindSubmatch(line)
	if res == nil {
		return nil, 0, 0, errEventStartRegexMismatch
	}
	comm := res[1]
	tid := 0
	pid, err := strconv.Atoi(string(res[2]))
	if err != nil {
		return nil, 0, 0, err
	}
	if res[3] != nil {
		tid, err = strconv.Atoi(string(res[3]))
		if err != nil {
			return nil, 0, 0, err
		}
	}
	return comm, pid, tid, nil
}

func parseEventEnd(line []byte) bool {
	return len(line) == 0
}

func parseStackFrame(line []byte) ([]byte, []byte, []byte, error) {
	res := reStackFrame.FindSubmatch(line)
	if res == nil {
		return nil, nil, nil, errStackFrameRegexMismatch
	}
	adr := res[1]
	sym := res[2]
	mod := res[3]
	return adr, sym, mod, nil
}
