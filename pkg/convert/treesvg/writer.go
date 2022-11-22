package treesvg

import (
	"bytes"
	"io"

	"github.com/sirupsen/logrus"
)

type writer struct {
	b *bytes.Buffer
}

type noopCloser struct {
	io.Writer
}

func (*noopCloser) Close() error { return nil }

func (w *writer) Open(name string) (io.WriteCloser, error) {
	logrus.Info("open ", name)
	return &noopCloser{w.b}, nil
}
