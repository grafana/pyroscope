// SPDX-License-Identifier: AGPL-3.0-only

package test

import (
	"bytes"
	"testing"

	"github.com/go-kit/log"
)

type TestingLogger struct {
	t testing.TB
}

func NewTestingLogger(t testing.TB) *TestingLogger {
	return &TestingLogger{t: t}
}

func (l *TestingLogger) Log(keyvals ...interface{}) error {
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
