// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package runutil

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/thanos-io/objstore/errutil"
)

// CloseWithErrCapture runs function and on error return error by argument including the given error (usually
// from caller function).
func CloseWithErrCapture(err *error, closer io.Closer, format string, a ...interface{}) {
	merr := errutil.MultiError{}

	merr.Add(*err)
	merr.Add(errors.Wrapf(closer.Close(), format, a...))

	*err = merr.Err()
}

// CloseWithLogOnErr is making sure we log every error, even those from best effort tiny closers.
func CloseWithLogOnErr(logger log.Logger, closer io.Closer, format string, a ...interface{}) {
	err := closer.Close()
	if err == nil {
		return
	}

	// Not a problem if it has been closed already.
	if errors.Is(err, os.ErrClosed) {
		return
	}

	if logger == nil {
		logger = log.NewLogfmtLogger(os.Stderr)
	}

	level.Warn(logger).Log("msg", "detected close error", "err", errors.Wrap(err, fmt.Sprintf(format, a...)))
}

// ExhaustCloseWithLogOnErr closes the io.ReadCloser with a log message on error but exhausts the reader before.
func ExhaustCloseWithLogOnErr(logger log.Logger, r io.ReadCloser, format string, a ...interface{}) {
	_, err := io.Copy(ioutil.Discard, r)
	if err != nil {
		level.Warn(logger).Log("msg", "failed to exhaust reader, performance may be impeded", "err", err)
	}

	CloseWithLogOnErr(logger, r, format, a...)
}
