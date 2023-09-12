// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/thanos-io/thanos/blob/main/pkg/testutil/e2eutil/copy.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Thanos Authors.

package test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/dskit/runutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func Copy(t testing.TB, src, dst string) {
	require.NoError(t, copyRecursive(src, dst))
}

func copyRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, relPath), os.ModePerm)
		}

		if !info.Mode().IsRegular() {
			return errors.Errorf("%s is not a regular file", path)
		}

		source, err := os.Open(filepath.Clean(path))
		if err != nil {
			return err
		}
		defer runutil.CloseWithErrCapture(&err, source, "close file")

		destination, err := os.Create(filepath.Join(dst, relPath))
		if err != nil {
			return err
		}
		defer runutil.CloseWithErrCapture(&err, destination, "close file")

		_, err = io.Copy(destination, source)
		return err
	})
}
