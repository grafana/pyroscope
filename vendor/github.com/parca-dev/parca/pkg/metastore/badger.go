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
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

// UUIDGenerator returns new UUIDs.
type UUIDGenerator interface {
	New() uuid.UUID
}

// RandomUUIDGenerator returns a new random UUID.
type RandomUUIDGenerator struct{}

// New returns a new UUID.
func (g *RandomUUIDGenerator) New() uuid.UUID {
	return uuid.New()
}

// NewRandomUUIDGenerator returns a new random UUID generator.
func NewRandomUUIDGenerator() UUIDGenerator {
	return &RandomUUIDGenerator{}
}

// Some tests need UUID generation to be predictable, so this generator just
// returns monotonically increasing UUIDs as if the UUID was a 16 byte integer.
// WARNING: THIS IS ONLY MEANT FOR TESTING.
type LinearUUIDGenerator struct {
	i uint64
}

// NewLinearUUIDGenerator returns a new LinearUUIDGenerator.
func NewLinearUUIDGenerator() UUIDGenerator {
	return &LinearUUIDGenerator{}
}

// New returns the next UUID according to the current count.
func (g *LinearUUIDGenerator) New() uuid.UUID {
	g.i++
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[8:], g.i)
	id, err := uuid.FromBytes(buf)
	if err != nil {
		panic(err)
	}

	return id
}

// BadgerMetastore is an implementation of the metastore using the badger KV
// store.
type BadgerMetastore struct {
	tracer trace.Tracer

	db *badger.DB

	uuidGenerator UUIDGenerator
}

type BadgerLogger struct {
	logger log.Logger
}

func (l *BadgerLogger) Errorf(f string, v ...interface{}) {
	level.Error(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

func (l *BadgerLogger) Warningf(f string, v ...interface{}) {
	level.Warn(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

func (l *BadgerLogger) Infof(f string, v ...interface{}) {
	level.Info(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

func (l *BadgerLogger) Debugf(f string, v ...interface{}) {
	level.Debug(l.logger).Log("msg", fmt.Sprintf(f, v...))
}

// NewBadgerMetastore returns a new BadgerMetastore with using in-memory badger
// instance.
func NewBadgerMetastore(
	logger log.Logger,
	reg prometheus.Registerer,
	tracer trace.Tracer,
	uuidGenerator UUIDGenerator,
) *BadgerMetastore {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(&BadgerLogger{logger: logger}))
	if err != nil {
		panic(err)
	}

	return &BadgerMetastore{
		db:            db,
		tracer:        tracer,
		uuidGenerator: uuidGenerator,
	}
}

// Close closes the badger store.
func (m *BadgerMetastore) Close() error {
	return m.db.Close()
}

// Ping returns an error if the metastore is not available.
func (m *BadgerMetastore) Ping() error {
	return nil
}

func (m *BadgerMetastore) GetStacktraceByKey(ctx context.Context, key []byte) (uuid.UUID, error) {
	var id uuid.UUID

	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrStacktraceNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			id, err = uuid.FromBytes(val)
			return err
		})
	})

	return id, err
}

func (m *BadgerMetastore) GetStacktraceByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Sample, error) {
	samples := map[string]*pb.Sample{}

	err := m.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			item, err := txn.Get(append([]byte(stacktraceIDPrefix), id[:]...))
			if err == badger.ErrKeyNotFound {
				return ErrStacktraceNotFound
			}
			if err != nil {
				return err
			}
			err = item.Value(func(val []byte) error {
				s := &pb.Sample{}
				err := s.UnmarshalVT(val)
				if err != nil {
					return err
				}
				samples[string(id)] = s
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return samples, err
}

func (m *BadgerMetastore) CreateStacktrace(ctx context.Context, key []byte, sample *pb.Sample) (uuid.UUID, error) {
	stacktraceID := m.uuidGenerator.New()

	buf, err := proto.Marshal(sample)
	if err != nil {
		return stacktraceID, err
	}

	err = m.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(append([]byte(stacktraceIDPrefix), stacktraceID[:]...), buf)
		if err != nil {
			return err
		}
		return txn.Set(key, stacktraceID[:])
	})

	return stacktraceID, err
}

// GetMappingsByIDs returns the mappings for the given IDs.
func (m *BadgerMetastore) GetMappingsByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Mapping, error) {
	mappings := map[string]*pb.Mapping{}
	err := m.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			item, err := txn.Get(append([]byte("mappings/by-id/"), id[:]...))
			if err != nil {
				return err
			}

			err = item.Value(func(val []byte) error {
				ma := &pb.Mapping{}
				err := ma.UnmarshalVT(val)
				if err != nil {
					return err
				}

				mappings[string(id)] = ma
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return mappings, err
}

// GetMappingByKey returns the mapping for the given key.
func (m *BadgerMetastore) GetMappingByKey(ctx context.Context, key *pb.Mapping) (*pb.Mapping, error) {
	ma := &pb.Mapping{}
	err := m.db.View(func(txn *badger.Txn) error {
		var err error
		item, err := txn.Get(MakeMappingKey(key))
		if err == badger.ErrKeyNotFound {
			return ErrMappingNotFound
		}
		if err != nil {
			return err
		}

		var mappingID uuid.UUID
		err = item.Value(func(val []byte) error {
			return mappingID.UnmarshalBinary(val)
		})
		if err != nil {
			return err
		}

		item, err = txn.Get(append([]byte("mappings/by-id/"), mappingID[:]...))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return ma.UnmarshalVT(val)
		})
	})
	if err != nil {
		return nil, err
	}

	return ma, nil
}

// CreateMapping creates a new mapping in the database.
func (m *BadgerMetastore) CreateMapping(ctx context.Context, mapping *pb.Mapping) ([]byte, error) {
	mappingID := m.uuidGenerator.New()
	mapping.Id = mappingID[:]
	buf, err := proto.Marshal(mapping)
	if err != nil {
		return nil, err
	}

	err = m.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(MakeMappingKey(mapping), mappingID[:])
		if err != nil {
			return err
		}

		return txn.Set(append([]byte("mappings/by-id/"), mappingID[:]...), buf)
	})

	return mapping.Id, err
}

// CreateFunction creates a new function in the database.
func (m *BadgerMetastore) CreateFunction(ctx context.Context, f *pb.Function) ([]byte, error) {
	functionID := m.uuidGenerator.New()
	f.Id = functionID[:]

	buf, err := proto.Marshal(f)
	if err != nil {
		return nil, err
	}

	err = m.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(MakeFunctionKey(f), f.Id)
		if err != nil {
			return err
		}

		return txn.Set(append([]byte("functions/by-id/"), f.Id...), buf)
	})

	return f.Id, err
}

// GetFunctionByKey returns the function for the given key.
func (m *BadgerMetastore) GetFunctionByKey(ctx context.Context, key *pb.Function) (*pb.Function, error) {
	f := &pb.Function{}
	err := m.db.View(func(txn *badger.Txn) error {
		var err error
		item, err := txn.Get(MakeFunctionKey(key))
		if err == badger.ErrKeyNotFound {
			return ErrFunctionNotFound
		}
		if err != nil {
			return fmt.Errorf("get function by key from store: %w", err)
		}

		var functionID uuid.UUID
		err = item.Value(func(val []byte) error {
			return functionID.UnmarshalBinary(val)
		})
		if err != nil {
			return err
		}

		item, err = txn.Get(append([]byte("functions/by-id/"), functionID[:]...))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return f.UnmarshalVT(val)
		})
	})
	if err != nil {
		return nil, err
	}

	return f, nil
}

// GetFunctions returns all functions in the database.
func (m *BadgerMetastore) GetFunctions(ctx context.Context) ([]*pb.Function, error) {
	var functions []*pb.Function
	err := m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10

		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte("functions/by-id/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				f := &pb.Function{}
				err := f.UnmarshalVT(val)
				if err != nil {
					return err
				}
				functions = append(functions, f)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return functions, err
}

// GetFunctionByID returns the function for the given ID.
func (m *BadgerMetastore) GetFunctionsByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Function, error) {
	functions := map[string]*pb.Function{}
	err := m.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			item, err := txn.Get(append([]byte("functions/by-id/"), id[:]...))
			if err != nil {
				return err
			}

			err = item.Value(func(val []byte) error {
				f := &pb.Function{}
				err := f.UnmarshalVT(val)
				if err != nil {
					return err
				}

				functions[string(id)] = f
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	return functions, err
}

// CreateLocationLines writes a set of lines related to a location to the database.
func (m *BadgerMetastore) CreateLocationLines(ctx context.Context, locID []byte, lines []LocationLine) error {
	l := &pb.LocationLines{
		Id:    locID,
		Lines: make([]*pb.Line, 0, len(lines)),
	}

	for _, line := range lines {
		l.Lines = append(l.Lines, &pb.Line{
			Line:       line.Line,
			FunctionId: line.Function.Id,
		})
	}

	buf, err := proto.Marshal(l)
	if err != nil {
		return err
	}

	return m.db.Update(func(txn *badger.Txn) error {
		return txn.Set(append([]byte("locations-lines/"), locID[:]...), buf)
	})
}

// GetLinesByLocationIDs returns the lines for the given location IDs.
func (m *BadgerMetastore) GetLinesByLocationIDs(ctx context.Context, ids ...[]byte) (
	map[string][]*pb.Line,
	[][]byte,
	error,
) {
	linesByLocation := map[string][]*pb.Line{}
	functionsSeen := map[string]struct{}{}
	functionsIDs := [][]byte{}
	err := m.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			item, err := txn.Get(append([]byte("locations-lines/"), id[:]...))
			if err == badger.ErrKeyNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to get location lines for ID %q: %w", id, err)
			}

			err = item.Value(func(val []byte) error {
				l := &pb.LocationLines{}
				err := l.UnmarshalVT(val)
				if err != nil {
					return fmt.Errorf("failed to unmarshal location lines for ID %q: %w", id, err)
				}

				for _, line := range l.Lines {
					if _, ok := functionsSeen[string(line.FunctionId)]; !ok {
						functionsIDs = append(functionsIDs, line.FunctionId)
						functionsSeen[string(line.FunctionId)] = struct{}{}
					}
				}

				linesByLocation[string(id)] = l.Lines
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return linesByLocation, functionsIDs, nil
}

func (m *BadgerMetastore) GetLocationByKey(ctx context.Context, key *Location) (*pb.Location, error) {
	l := &pb.Location{}
	err := m.db.View(func(txn *badger.Txn) error {
		var err error
		item, err := txn.Get(MakeLocationKey(key))
		if err == badger.ErrKeyNotFound {
			return ErrLocationNotFound
		}
		if err != nil {
			return err
		}

		var locationID uuid.UUID
		err = item.Value(func(val []byte) error {
			return locationID.UnmarshalBinary(val)
		})
		if err != nil {
			return err
		}

		item, err = txn.Get(append([]byte("locations/by-id/"), locationID[:]...))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return l.UnmarshalVT(val)
		})
	})
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (m *BadgerMetastore) GetLocationsByIDs(ctx context.Context, ids ...[]byte) (
	map[string]*pb.Location,
	[][]byte,
	error,
) {
	locations := map[string]*pb.Location{}
	mappingsSeen := map[string]struct{}{}
	mappingIDs := [][]byte{}
	err := m.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			item, err := txn.Get(append([]byte("locations/by-id/"), id[:]...))
			if err != nil {
				return err
			}

			err = item.Value(func(val []byte) error {
				l := &pb.Location{}
				err := l.UnmarshalVT(val)
				if err != nil {
					return err
				}

				if len(l.MappingId) > 0 && !bytes.Equal(l.MappingId, uuid.Nil[:]) {
					if _, ok := mappingsSeen[string(l.MappingId)]; !ok {
						mappingIDs = append(mappingIDs, l.MappingId)
						mappingsSeen[string(l.MappingId)] = struct{}{}
					}
				}

				locations[string(id)] = l
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return locations, mappingIDs, nil
}

func (m *BadgerMetastore) CreateLocation(ctx context.Context, l *Location) ([]byte, error) {
	id := m.uuidGenerator.New()
	loc := &pb.Location{
		Id:       id[:],
		Address:  l.Address,
		IsFolded: l.IsFolded,
	}

	if l.Mapping != nil {
		loc.MappingId = l.Mapping.Id
	}

	buf, err := proto.Marshal(loc)
	if err != nil {
		return nil, err
	}

	err = m.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(MakeLocationKey(l), id[:])
		if err != nil {
			return err
		}

		if l.Address != uint64(0) && l.Mapping != nil && len(l.Lines) == 0 {
			err := txn.Set(append([]byte("locations-unsymbolized/by-id/"), id[:]...), id[:])
			if err != nil {
				return err
			}
		}

		return txn.Set(append([]byte("locations/by-id/"), id[:]...), buf)
	})
	if err != nil {
		return nil, err
	}

	if len(l.Lines) > 0 {
		return loc.Id, m.CreateLocationLines(ctx, loc.Id, l.Lines)
	}

	return loc.Id, nil
}

func (m *BadgerMetastore) GetSymbolizableLocations(ctx context.Context) ([]*pb.Location, [][]byte, error) {
	ids := [][]byte{}
	err := m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte("locations-unsymbolized/by-id/")
		prefixLen := len(prefix)

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			ids = append(ids, it.Item().KeyCopy(nil)[prefixLen:])
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	locsByIDs, mappingIDs, err := m.GetLocationsByIDs(ctx, ids...)
	if err != nil {
		return nil, nil, err
	}

	locs := make([]*pb.Location, 0, len(locsByIDs))
	for _, loc := range locsByIDs {
		locs = append(locs, loc)
	}

	return locs, mappingIDs, nil
}

func (m *BadgerMetastore) GetLocations(ctx context.Context) ([]*pb.Location, [][]byte, error) {
	return m.getLocations(ctx, []byte("locations/by-id/"))
}

func (m *BadgerMetastore) getLocations(ctx context.Context, prefix []byte) ([]*pb.Location, [][]byte, error) {
	locations := []*pb.Location{}
	mappingsSeen := map[string]struct{}{}
	mappingIDs := [][]byte{}
	err := m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10

		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				l := &pb.Location{}
				err := l.UnmarshalVT(val)
				if err != nil {
					return err
				}

				if len(l.MappingId) > 0 && !bytes.Equal(l.MappingId, uuid.Nil[:]) {
					if _, ok := mappingsSeen[string(l.MappingId)]; !ok {
						mappingIDs = append(mappingIDs, l.MappingId)
						mappingsSeen[string(l.MappingId)] = struct{}{}
					}
				}

				locations = append(locations, l)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return locations, mappingIDs, err
}

func (m *BadgerMetastore) Symbolize(ctx context.Context, l *Location) error {
	for _, l := range l.Lines {
		functionID, err := m.getOrCreateFunction(ctx, l.Function)
		if err != nil {
			return fmt.Errorf("get or create function: %w", err)
		}
		l.Function.Id = functionID[:]
	}

	if err := m.CreateLocationLines(ctx, l.ID[:], l.Lines); err != nil {
		return fmt.Errorf("create lines: %w", err)
	}

	return m.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(append([]byte("locations-unsymbolized/by-id/"), l.ID[:]...))
	})
}

func (m *BadgerMetastore) getOrCreateFunction(ctx context.Context, f *pb.Function) ([]byte, error) {
	fn, err := m.GetFunctionByKey(ctx, f)
	if err == nil {
		return fn.Id, nil
	}
	if err != nil && err != ErrFunctionNotFound {
		return nil, fmt.Errorf("get function by key: %w", err)
	}

	id, err := m.CreateFunction(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("create function: %w", err)
	}

	return id, nil
}
