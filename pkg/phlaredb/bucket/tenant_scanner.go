// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/users_scanner.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucket

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/grafana/pyroscope/pkg/objstore"
)

// AllTenants returns true to each call and should be used whenever the UsersScanner should not filter out
// any user due to sharding.
func AllTenants(_ string) (bool, error) {
	return true, nil
}

type TenantsScanner struct {
	bucketClient objstore.Bucket
	logger       log.Logger
	isOwned      func(userID string) (bool, error)
}

func NewTenantsScanner(bucketClient objstore.Bucket, isOwned func(userID string) (bool, error), logger log.Logger) *TenantsScanner {
	return &TenantsScanner{
		bucketClient: bucketClient,
		logger:       logger,
		isOwned:      isOwned,
	}
}

// ScanTenants returns a fresh list of users found in the storage, that are not marked for deletion,
// and list of users marked for deletion.
//
// If sharding is enabled, returned lists contains only the users owned by this instance.
func (s *TenantsScanner) ScanTenants(ctx context.Context) (users, markedForDeletion []string, err error) {
	users, err = ListUsers(ctx, s.bucketClient)
	if err != nil {
		return nil, nil, err
	}

	// Check users for being owned by instance, and split users into non-deleted and deleted.
	// We do these checks after listing all users, to improve cacheability of Iter (result is only cached at the end of Iter call).
	for ix := 0; ix < len(users); {
		userID := users[ix]

		// Check if it's owned by this instance.
		owned, err := s.isOwned(userID)
		if err != nil {
			level.Warn(s.logger).Log("msg", "unable to check if user is owned by this shard", "tenant", userID, "err", err)
		} else if !owned {
			users = append(users[:ix], users[ix+1:]...)
			continue
		}

		deletionMarkExists, err := TenantDeletionMarkExists(ctx, s.bucketClient, userID)
		if err != nil {
			level.Warn(s.logger).Log("msg", "unable to check if user is marked for deletion", "tenant", userID, "err", err)
		} else if deletionMarkExists {
			users = append(users[:ix], users[ix+1:]...)
			markedForDeletion = append(markedForDeletion, userID)
			continue
		}

		ix++
	}

	return users, markedForDeletion, nil
}
