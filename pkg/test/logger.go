// SPDX-License-Identifier: AGPL-3.0-only

package test

import (
	"bytes"
	"testing"

	"github.com/go-kit/log"
)

type testingLogger struct {
	t testing.TB
}

func NewTestingLogger(t testing.TB) log.Logger {
	return &testingLogger{
		t: t,
	}
}

func (l *testingLogger) Log(keyvals ...interface{}) error {
	l.t.Helper()
	buf := bytes.NewBuffer(nil)
	lf := log.NewLogfmtLogger(buf)
	lf.Log(keyvals...)
	bs := buf.Bytes()
	if len(bs) > 0 && bs[len(bs)-1] == '\n' {
		bs = bs[:len(bs)-1]
	}
	l.t.Log(string(bs))
	return nil
}
