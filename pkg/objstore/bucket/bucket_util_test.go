// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/bucket_util_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucket

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
)

func TestDeletePrefix(t *testing.T) {
	mem := objstore.NewInMemBucket()

	require.NoError(t, mem.Upload(context.Background(), "obj", strings.NewReader("hello")))
	require.NoError(t, mem.Upload(context.Background(), "prefix/1", strings.NewReader("hello")))
	require.NoError(t, mem.Upload(context.Background(), "prefix/2", strings.NewReader("hello")))
	require.NoError(t, mem.Upload(context.Background(), "prefix/sub1/3", strings.NewReader("hello")))
	require.NoError(t, mem.Upload(context.Background(), "prefix/sub2/4", strings.NewReader("hello")))
	require.NoError(t, mem.Upload(context.Background(), "outside/obj", strings.NewReader("hello")))

	del, err := DeletePrefix(context.Background(), mem, "prefix", log.NewNopLogger())
	require.NoError(t, err)
	assert.Equal(t, 4, del)
	assert.Equal(t, 2, len(mem.Objects()))
}
