package filesystem

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
)

func TestIter(t *testing.T) {
	bkt, err := NewBucket(t.TempDir())
	require.NoError(t, err)
	defer bkt.Close()

	buff := bytes.NewBufferString("foo")
	require.NoError(t, bkt.Upload(context.Background(), "foo/bar/buz1", buff))
	require.NoError(t, bkt.Upload(context.Background(), "foo/bar/buz2", buff))
	require.NoError(t, bkt.Upload(context.Background(), "foo/ba/buzz3", buff))
	require.NoError(t, bkt.Upload(context.Background(), "foo/buzz4", buff))
	require.NoError(t, bkt.Upload(context.Background(), "foo/buzz5", buff))
	require.NoError(t, bkt.Upload(context.Background(), "foo6", buff))

	for _, tc := range []struct {
		prefix   string
		expected []string
		options  []objstore.IterOption
	}{
		{
			prefix:   "foo/",
			expected: []string{"foo/ba/", "foo/bar/", "foo/buzz4", "foo/buzz5"},
			options:  []objstore.IterOption{},
		},
		{
			prefix:   "foo/",
			expected: []string{"foo/ba/buzz3", "foo/bar/buz1", "foo/bar/buz2", "foo/buzz4", "foo/buzz5"},
			options:  []objstore.IterOption{objstore.WithRecursiveIter},
		},
		{
			prefix:   "foo/ba",
			expected: []string{"foo/ba/buzz3"},
			options:  []objstore.IterOption{objstore.WithRecursiveIter},
		},
		{
			prefix:   "foo/ba/",
			expected: []string{"foo/ba/buzz3"},
			options:  []objstore.IterOption{objstore.WithRecursiveIter},
		},
		{
			prefix:  "foo/b",
			options: []objstore.IterOption{objstore.WithRecursiveIter},
		},
		{
			prefix:   "foo",
			expected: []string{"foo/ba/", "foo/bar/", "foo/buzz4", "foo/buzz5"},
			options:  []objstore.IterOption{},
		},
		{
			prefix:   "foo",
			expected: []string{"foo/ba/buzz3", "foo/bar/buz1", "foo/bar/buz2", "foo/buzz4", "foo/buzz5"},
			options:  []objstore.IterOption{objstore.WithRecursiveIter},
		},
		{
			prefix:  "fo",
			options: []objstore.IterOption{},
		},
		{
			prefix:  "fo",
			options: []objstore.IterOption{objstore.WithRecursiveIter},
		},
		{
			prefix:   "",
			expected: []string{"foo/", "foo6"},
			options:  []objstore.IterOption{},
		},
		{
			prefix:   "",
			expected: []string{"foo/ba/buzz3", "foo/bar/buz1", "foo/bar/buz2", "foo/buzz4", "foo/buzz5", "foo6"},
			options:  []objstore.IterOption{objstore.WithRecursiveIter},
		},
		{
			prefix:   "foo",
			expected: []string{"foo/", "foo6"},
			options:  []objstore.IterOption{objstore.WithoutApendingDirDelim},
		},
		{
			prefix:   "f",
			expected: []string{"foo/", "foo6"},
			options:  []objstore.IterOption{objstore.WithoutApendingDirDelim},
		},
		{
			prefix:   "foo/ba",
			expected: []string{"foo/ba/", "foo/bar/"},
			options:  []objstore.IterOption{objstore.WithoutApendingDirDelim},
		},
		{
			prefix:   "foo/ba",
			expected: []string{"foo/ba/buzz3", "foo/bar/buz1", "foo/bar/buz2"},
			options:  []objstore.IterOption{objstore.WithoutApendingDirDelim, objstore.WithRecursiveIter},
		},
		{
			prefix:   "fo",
			expected: []string{"foo/ba/buzz3", "foo/bar/buz1", "foo/bar/buz2", "foo/buzz4", "foo/buzz5", "foo6"},
			options:  []objstore.IterOption{objstore.WithoutApendingDirDelim, objstore.WithRecursiveIter},
		},
	} {
		tc := tc
		t.Run(tc.prefix, func(t *testing.T) {
			var keys []string
			err = bkt.Iter(context.Background(), tc.prefix, func(key string) error {
				keys = append(keys, key)
				return nil
			}, tc.options...)
			require.NoError(t, err)
			require.Equal(t, tc.expected, keys)
		})
	}
}
