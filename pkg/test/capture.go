package test

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type CapturedOutput struct {
	stdoutBuf bytes.Buffer
	stderrBuf bytes.Buffer

	wg                         sync.WaitGroup
	stdoutReader, stdoutWriter *os.File
	stderrReader, stderrWriter *os.File
}

// CaptureOutput replaces os.Stdout and os.Stderr with new pipes, that will
// write output to buffers. Buffers are accessible by calling Done on returned
// struct.
//
// os.Stdout and os.Stderr must be reverted to previous values manually.
func CaptureOutput(t *testing.T) *CapturedOutput {
	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)

	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = stdoutW
	os.Stderr = stderrW

	co := &CapturedOutput{
		stdoutReader: stdoutR,
		stdoutWriter: stdoutW,
		stderrReader: stderrR,
		stderrWriter: stderrW,
	}
	co.wg.Add(1)
	go func() {
		defer co.wg.Done()
		_, _ = io.Copy(&co.stdoutBuf, stdoutR)
	}()

	co.wg.Add(1)
	go func() {
		defer co.wg.Done()
		_, _ = io.Copy(&co.stderrBuf, stderrR)
	}()

	return co
}

// Done waits until all captured output has been written to buffers,
// and then returns the buffers.
func (co *CapturedOutput) Done() (stdout string, stderr string) {
	// we need to close writers for readers to stop
	_ = co.stdoutWriter.Close()
	_ = co.stderrWriter.Close()

	co.wg.Wait()

	return co.stdoutBuf.String(), co.stderrBuf.String()
}
