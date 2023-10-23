// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/users_scanner_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucket

import (
	"context"
	"errors"
	"path"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore"
)

func TestUsersScanner_ScanUsers_ShouldReturnedOwnedUsersOnly(t *testing.T) {
	bucketClient := &objstore.ClientMock{}
	bucketClient.MockIter("", []string{"user-1", "user-2", "user-3", "user-4"}, nil)
	bucketClient.MockExists(path.Join("user-1", "phlaredb/", TenantDeletionMarkPath), false, nil)
	bucketClient.MockExists(path.Join("user-3", "phlaredb/", TenantDeletionMarkPath), true, nil)

	isOwned := func(userID string) (bool, error) {
		return userID == "user-1" || userID == "user-3", nil
	}

	s := NewTenantsScanner(bucketClient, isOwned, log.NewNopLogger())
	actual, deleted, err := s.ScanTenants(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"user-1"}, actual)
	assert.Equal(t, []string{"user-3"}, deleted)
}

func TestUsersScanner_ScanUsers_ShouldReturnUsersForWhichOwnerCheckOrTenantDeletionCheckFailed(t *testing.T) {
	expected := []string{"user-1", "user-2"}

	bucketClient := &objstore.ClientMock{}
	bucketClient.MockIter("", expected, nil)
	bucketClient.MockExists(path.Join("user-1", "phlaredb/", TenantDeletionMarkPath), false, nil)
	bucketClient.MockExists(path.Join("user-2", "phlaredb/", TenantDeletionMarkPath), false, errors.New("fail"))

	isOwned := func(userID string) (bool, error) {
		return false, errors.New("failed to check if user is owned")
	}

	s := NewTenantsScanner(bucketClient, isOwned, log.NewNopLogger())
	actual, deleted, err := s.ScanTenants(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Empty(t, deleted)
}

func TestUsersScanner_ScanUsers_ShouldNotReturnPrefixedUsedByPyroscopeInternals(t *testing.T) {
	bucketClient := &objstore.ClientMock{}
	bucketClient.MockIter("", []string{"user-1", "user-2", PyroscopeInternalsPrefix}, nil)
	bucketClient.MockExists(path.Join("user-1", "phlaredb/", TenantDeletionMarkPath), false, nil)
	bucketClient.MockExists(path.Join("user-2", "phlaredb/", TenantDeletionMarkPath), false, nil)

	s := NewTenantsScanner(bucketClient, AllTenants, log.NewNopLogger())
	actual, _, err := s.ScanTenants(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"user-1", "user-2"}, actual)
}
