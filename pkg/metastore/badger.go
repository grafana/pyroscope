// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/parca-dev/parca/blob/main/pkg/metastore/badger.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Parca Authors.

package metastore

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/metastore"
)

// BadgerMetastore is an implementation of the metastore using the badger KV
// store.
type BadgerMetastore struct {
	tracer trace.Tracer

	db *badger.DB

	pb.UnimplementedMetastoreServiceServer
}

type badgerLogger struct {
	logger log.Logger
}

func (l *badgerLogger) Errorf(f string, v ...interface{}) {
	level.Error(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

func (l *badgerLogger) Warningf(f string, v ...interface{}) {
	level.Warn(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

func (l *badgerLogger) Infof(f string, v ...interface{}) {
	level.Info(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

func (l *badgerLogger) Debugf(f string, v ...interface{}) {
	level.Debug(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

var _ pb.MetastoreServiceServer = &BadgerMetastore{}

// NewBadgerMetastore returns a new BadgerMetastore with using in-memory badger
// instance.
func NewBadgerMetastore(
	logger log.Logger,
	reg prometheus.Registerer,
	tracer trace.Tracer,
	dataPath string,
) (*BadgerMetastore, error) {
	opts := badger.DefaultOptions(dataPath).WithLogger(&badgerLogger{logger: logger})
	if dataPath == "" {
		opts = opts.WithInMemory(true)
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &BadgerMetastore{
		db:     db,
		tracer: tracer,
	}, nil
}

func (m *BadgerMetastore) Mappings(ctx context.Context, r *pb.MappingsRequest) (*pb.MappingsResponse, error) {
	res := &pb.MappingsResponse{
		Mappings: make([]*pb.Mapping, 0, len(r.MappingIds)),
	}

	mappingKeys := make([][]byte, 0, len(r.MappingIds))
	for _, id := range r.MappingIds {
		mappingKeys = append(mappingKeys, []byte(metastore.MakeMappingKeyWithID(id)))
	}

	err := m.db.View(func(txn *badger.Txn) error {
		for _, mappingKey := range mappingKeys {
			item, err := txn.Get(mappingKey)
			if err != nil {
				return err
			}

			err = item.Value(func(val []byte) error {
				mapping := &pb.Mapping{}
				err := mapping.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.Mappings = append(res.Mappings, mapping)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return res, err
}

func (m *BadgerMetastore) GetOrCreateMappings(ctx context.Context, r *pb.GetOrCreateMappingsRequest) (*pb.GetOrCreateMappingsResponse, error) {
	res := &pb.GetOrCreateMappingsResponse{
		Mappings: make([]*pb.Mapping, 0, len(r.Mappings)),
	}

	mappingKeys := make([]string, 0, len(r.Mappings))
	for _, id := range r.Mappings {
		mappingKeys = append(mappingKeys, metastore.MakeMappingKey(id))
	}

	err := m.db.Update(func(txn *badger.Txn) error {
		for i, mappingKey := range mappingKeys {
			item, err := txn.Get([]byte(mappingKey))
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}

			if err == badger.ErrKeyNotFound {
				mapping := r.Mappings[i]
				mapping.Id = metastore.MappingIDFromKey(mappingKey)
				b, err := mapping.MarshalVT()
				if err != nil {
					return err
				}
				if err := txn.Set([]byte(mappingKey), b); err != nil {
					return err
				}
				res.Mappings = append(res.Mappings, mapping)
				continue
			}

			err = item.Value(func(val []byte) error {
				mapping := &pb.Mapping{}
				err := mapping.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.Mappings = append(res.Mappings, mapping)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return res, err
}

func (m *BadgerMetastore) Functions(ctx context.Context, r *pb.FunctionsRequest) (*pb.FunctionsResponse, error) {
	res := &pb.FunctionsResponse{
		Functions: make([]*pb.Function, 0, len(r.FunctionIds)),
	}

	functionKeys := make([][]byte, 0, len(r.FunctionIds))
	for _, id := range r.FunctionIds {
		functionKeys = append(functionKeys, []byte(metastore.MakeFunctionKeyWithID(id)))
	}

	err := m.db.View(func(txn *badger.Txn) error {
		for _, functionKey := range functionKeys {
			item, err := txn.Get(functionKey)
			if err != nil {
				return err
			}

			err = item.Value(func(val []byte) error {
				function := &pb.Function{}
				err := function.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.Functions = append(res.Functions, function)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return res, err
}

func (m *BadgerMetastore) GetOrCreateFunctions(ctx context.Context, r *pb.GetOrCreateFunctionsRequest) (*pb.GetOrCreateFunctionsResponse, error) {
	res := &pb.GetOrCreateFunctionsResponse{
		Functions: make([]*pb.Function, 0, len(r.Functions)),
	}

	functionKeys := make([]string, 0, len(r.Functions))
	for _, function := range r.Functions {
		functionKeys = append(functionKeys, metastore.MakeFunctionKey(function))
	}

	err := m.db.Update(func(txn *badger.Txn) error {
		for i, functionKey := range functionKeys {
			item, err := txn.Get([]byte(functionKey))
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}

			if err == badger.ErrKeyNotFound {
				function := r.Functions[i]
				function.Id = metastore.FunctionIDFromKey(functionKey)
				b, err := function.MarshalVT()
				if err != nil {
					return err
				}
				if err := txn.Set([]byte(functionKey), b); err != nil {
					return err
				}
				res.Functions = append(res.Functions, function)
				continue
			}

			err = item.Value(func(val []byte) error {
				function := &pb.Function{}
				err := function.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.Functions = append(res.Functions, function)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return res, err
}

func (m *BadgerMetastore) LocationLines(ctx context.Context, r *pb.LocationLinesRequest) (*pb.LocationLinesResponse, error) {
	res := &pb.LocationLinesResponse{
		LocationLines: make([]*pb.LocationLines, 0, len(r.LocationIds)),
	}

	locationLineKeys := make([][]byte, 0, len(r.LocationIds))
	for _, id := range r.LocationIds {
		locationLineKeys = append(locationLineKeys, []byte(metastore.MakeLocationLinesKeyWithID(id)))
	}

	err := m.db.View(func(txn *badger.Txn) error {
		for _, locationLineKey := range locationLineKeys {
			item, err := txn.Get(locationLineKey)
			if err != nil {
				return err
			}

			err = item.Value(func(val []byte) error {
				locationLines := &pb.LocationLines{}
				err := locationLines.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.LocationLines = append(res.LocationLines, locationLines)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return res, err
}

func (m *BadgerMetastore) Locations(ctx context.Context, r *pb.LocationsRequest) (*pb.LocationsResponse, error) {
	res := &pb.LocationsResponse{
		Locations: make([]*pb.Location, 0, len(r.LocationIds)),
	}

	locationKeys := make([][]byte, 0, len(r.LocationIds))
	for _, id := range r.LocationIds {
		locationKeys = append(locationKeys, []byte(metastore.MakeLocationKeyWithID(id)))
	}

	err := m.db.View(func(txn *badger.Txn) error {
		var err error
		res.Locations, err = m.locations(ctx, txn, res.Locations, locationKeys)
		return err
	})

	return res, err
}

func (m *BadgerMetastore) locations(ctx context.Context, txn *badger.Txn, locations []*pb.Location, locationKeys [][]byte) ([]*pb.Location, error) {
	for _, locationKey := range locationKeys {
		item, err := txn.Get(locationKey)
		if err != nil {
			return nil, err
		}

		err = item.Value(func(val []byte) error {
			location := &pb.Location{}
			err := location.UnmarshalVT(val)
			if err != nil {
				return err
			}

			locations = append(locations, location)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return locations, nil
}

func (m *BadgerMetastore) GetOrCreateLocations(ctx context.Context, r *pb.GetOrCreateLocationsRequest) (*pb.GetOrCreateLocationsResponse, error) {
	res := &pb.GetOrCreateLocationsResponse{
		Locations: make([]*pb.Location, 0, len(r.Locations)),
	}

	locationKeys := make([]string, 0, len(r.Locations))
	for _, location := range r.Locations {
		locationKeys = append(locationKeys, metastore.MakeLocationKey(location))
	}

	symbolizedLocationKeys := make([]string, 0, len(r.Locations))
	symbolizedLocations := make([]*pb.Location, 0, len(r.Locations))

	err := m.db.Update(func(txn *badger.Txn) error {
		for i, locationKey := range locationKeys {
			item, err := txn.Get([]byte(locationKey))
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}

			if err == badger.ErrKeyNotFound {
				location := r.Locations[i]
				location.Id = metastore.LocationIDFromKey(locationKey)
				b, err := location.MarshalVT()
				if err != nil {
					return err
				}
				if err := txn.Set([]byte(locationKey), b); err != nil {
					return err
				}
				res.Locations = append(res.Locations, location)

				if location.MappingId != "" && location.Address != 0 && (location.Lines == nil || len(location.Lines.Entries) == 0) {
					unsymbolizableKey := metastore.MakeUnsymbolizedLocationKeyWithID(location.Id)
					if err := txn.Set([]byte(unsymbolizableKey), []byte{}); err != nil {
						return err
					}
					continue
				}

				symbolizedLocationKeys = append(symbolizedLocationKeys, location.Id)
				symbolizedLocations = append(symbolizedLocations, location)

				continue
			}

			err = item.Value(func(val []byte) error {
				location := &pb.Location{}
				err := location.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.Locations = append(res.Locations, location)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return m.createLocationLines(ctx, txn, symbolizedLocationKeys, symbolizedLocations)
	})

	return res, err
}

func (m *BadgerMetastore) UnsymbolizedLocations(ctx context.Context, r *pb.UnsymbolizedLocationsRequest) (*pb.UnsymbolizedLocationsResponse, error) {
	var locations []*pb.Location

	err := m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		locationKeys := [][]byte{}
		prefix := []byte(metastore.MakeUnsymbolizedLocationKeyWithID(""))
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := metastore.MakeLocationKeyWithID(metastore.LocationIDFromUnsymbolizedKey(string(it.Item().Key())))
			locationKeys = append(locationKeys, []byte(key))
		}

		locations = make([]*pb.Location, 0, len(locationKeys))
		var err error
		locations, err = m.locations(ctx, txn, locations, locationKeys)

		return err
	})
	if err != nil {
		return nil, err
	}

	return &pb.UnsymbolizedLocationsResponse{
		Locations: locations,
	}, nil
}

func (m *BadgerMetastore) CreateLocationLines(ctx context.Context, r *pb.CreateLocationLinesRequest) (*pb.CreateLocationLinesResponse, error) {
	locationIDs := make([]string, 0, len(r.Locations))
	for _, location := range r.Locations {
		locationIDs = append(locationIDs, metastore.MakeLocationID(location))
	}

	err := m.db.Update(func(txn *badger.Txn) error {
		return m.createLocationLines(ctx, txn, locationIDs, r.Locations)
	})
	if err != nil {
		return nil, err
	}

	return &pb.CreateLocationLinesResponse{}, nil
}

func (m *BadgerMetastore) createLocationLines(ctx context.Context, txn *badger.Txn, locationIDs []string, locations []*pb.Location) error {
	for i, locationID := range locationIDs {
		b, err := locations[i].Lines.MarshalVT()
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(metastore.MakeLocationLinesKeyWithID(locationID)), b); err != nil {
			return err
		}

		if err := txn.Delete([]byte(metastore.MakeUnsymbolizedLocationKeyWithID(locationID))); err != nil {
			return err
		}
	}
	return nil
}

func (m *BadgerMetastore) GetOrCreateStacktraces(ctx context.Context, r *pb.GetOrCreateStacktracesRequest) (*pb.GetOrCreateStacktracesResponse, error) {
	res := &pb.GetOrCreateStacktracesResponse{
		Stacktraces: make([]*pb.Stacktrace, 0, len(r.Stacktraces)),
	}

	stacktraceKeys := make([]string, 0, len(r.Stacktraces))
	for _, stacktrace := range r.Stacktraces {
		stacktraceKeys = append(stacktraceKeys, metastore.MakeStacktraceKey(stacktrace))
	}

	err := m.db.Update(func(txn *badger.Txn) error {
		for i, stacktraceKey := range stacktraceKeys {
			item, err := txn.Get([]byte(stacktraceKey))
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}

			if err == badger.ErrKeyNotFound {
				stacktrace := r.Stacktraces[i]
				stacktrace.Id = metastore.StacktraceIDFromKey(stacktraceKey)
				b, err := stacktrace.MarshalVT()
				if err != nil {
					return err
				}
				if err := txn.Set([]byte(stacktraceKey), b); err != nil {
					return err
				}
				res.Stacktraces = append(res.Stacktraces, stacktrace)
				continue
			}

			err = item.Value(func(val []byte) error {
				stacktrace := &pb.Stacktrace{}
				err := stacktrace.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.Stacktraces = append(res.Stacktraces, stacktrace)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return res, err
}

func (m *BadgerMetastore) Stacktraces(ctx context.Context, r *pb.StacktracesRequest) (*pb.StacktracesResponse, error) {
	res := &pb.StacktracesResponse{
		Stacktraces: make([]*pb.Stacktrace, 0, len(r.StacktraceIds)),
	}

	stacktraceKeys := make([][]byte, 0, len(r.StacktraceIds))
	for _, id := range r.StacktraceIds {
		stacktraceKeys = append(stacktraceKeys, []byte(metastore.MakeStacktraceKeyWithID(id)))
	}

	err := m.db.View(func(txn *badger.Txn) error {
		for _, stacktraceKey := range stacktraceKeys {
			item, err := txn.Get(stacktraceKey)
			if err != nil {
				return err
			}

			err = item.Value(func(val []byte) error {
				stacktrace := &pb.Stacktrace{}
				err := stacktrace.UnmarshalVT(val)
				if err != nil {
					return err
				}

				res.Stacktraces = append(res.Stacktraces, stacktrace)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return res, err
}
