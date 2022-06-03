// Copyright 2021 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metastore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

var _ ProfileMetaStore = &RemoteMetaStore{}

type RemoteMetaStore struct {
	*sqlMetaStore
}

// NewRemoteMetaStore creates a sql metastore with given remote database connection.
func NewRemoteMetaStore(reg prometheus.Registerer, db *sql.DB) (*RemoteMetaStore, error) {
	remoteDB := &RemoteMetaStore{
		sqlMetaStore: &sqlMetaStore{
			db:    db,
			cache: newMetaStoreCache(reg),
		},
	}

	if err := remoteDB.migrate(); err != nil {
		return nil, fmt.Errorf("migrations failed: %w", err)
	}

	return remoteDB, nil
}

func (r RemoteMetaStore) GetStacktraceByKey(ctx context.Context, key []byte) (uuid.UUID, error) {
	panic("implement me")
}

func (r RemoteMetaStore) GetStacktraceByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Sample, error) {
	panic("implement me")
}

func (r RemoteMetaStore) CreateStacktrace(ctx context.Context, key []byte, sample *pb.Sample) (uuid.UUID, error) {
	panic("implement me")
}
