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
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

type sqlMetaStore struct {
	db     *sql.DB
	cache  *metaStoreCache
	tracer trace.Tracer
}

func (s *sqlMetaStore) migrate() error {
	// Most of the tables have started their lives as representation of pprof data types.
	// Find detailed information in https://github.com/google/pprof/blob/master/proto/README.md
	tables := []string{
		"PRAGMA foreign_keys = ON",
		`CREATE TABLE "mappings" (
			"id" TEXT NOT NULL PRIMARY KEY,
			"start"           	INT64,
			"limit"          	INT64,
			"offset"          	INT64,
			"file"           	TEXT,
			"build_id"         	TEXT,
			"has_functions"    	BOOLEAN,
			"has_filenames"    	BOOLEAN,
			"has_line_numbers"  BOOLEAN,
			"has_inline_frames" BOOLEAN,
			"size"				INT64,
			"build_id_or_file"	TEXT,
			UNIQUE (size, offset, build_id_or_file)
		);`,
		`CREATE INDEX idx_mapping_key ON mappings (size, offset, build_id_or_file);`,
		`CREATE TABLE "functions" (
			"id" TEXT NOT NULL PRIMARY KEY,
			"name"       	TEXT,
			"system_name" 	TEXT,
			"filename"   	TEXT,
			"start_line"  	INT64,
			UNIQUE (name, system_name, filename, start_line)
		);`,
		`CREATE INDEX idx_function_key ON functions (start_line, name, system_name, filename);`,
		`CREATE TABLE "lines" (
			"location_id" TEXT NOT NULL,
			"function_id" TEXT NOT NULL,
			"line" 		  INT64,
			FOREIGN KEY (function_id) REFERENCES functions (id),
			FOREIGN KEY (location_id) REFERENCES locations (id),
			UNIQUE (location_id, function_id, line)
		);`,
		`CREATE INDEX idx_line_location ON lines (location_id);`,
		`CREATE TABLE "locations" (
			"id" TEXT NOT NULL PRIMARY KEY,
			"mapping_id"  			TEXT,
			"address"  				INT64,
			"is_folded" 			BOOLEAN,
			"normalized_address"	INT64,
			"lines"					TEXT,
			FOREIGN KEY (mapping_id) REFERENCES mappings (id),
			UNIQUE (mapping_id, is_folded, normalized_address, lines)
		);`,
		`CREATE INDEX idx_location_key ON locations (normalized_address, mapping_id, is_folded, lines);`,
	}

	for _, t := range tables {
		statement, err := s.db.Prepare(t)
		if err != nil {
			return err
		}

		if _, err := statement.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqlMetaStore) GetLocationByKey(ctx context.Context, lkey *Location) (*pb.Location, error) {
	k := MakeSQLLocationKey(lkey)

	l, found, err := s.cache.getLocationByKey(ctx, k)
	if err != nil {
		return nil, fmt.Errorf("get location by key from cache: %w", err)
	}
	if !found {
		var (
			id      string
			address int64
			err     error
		)

		l = &pb.Location{}

		if k.MappingID != uuid.Nil {
			err = s.db.QueryRowContext(ctx,
				`SELECT "id", "address"
					FROM "locations" l
					WHERE normalized_address=?
					  AND is_folded=?
					  AND lines=?
					  AND mapping_id=? `,
				int64(k.Address), k.IsFolded, k.Lines, k.MappingID,
			).Scan(&id, &address)
		} else {
			err = s.db.QueryRowContext(ctx,
				`SELECT "id", "address"
					FROM "locations" l
					WHERE normalized_address=?
					  AND mapping_id IS NULL
					  AND is_folded=?
					  AND lines=?`,
				int64(k.Address), k.IsFolded, k.Lines,
			).Scan(&id, &address)
		}
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrLocationNotFound
			}
			return nil, err
		}
		lID, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("parse location id: %w", err)
		}

		l.Id = lID[:]
		l.Address = uint64(address)
		l.IsFolded = k.IsFolded
		l.MappingId = k.MappingID[:]

		err = s.cache.setLocationByKey(ctx, k, l)
		if err != nil {
			return nil, fmt.Errorf("set location by key in cache: %w", err)
		}
	}

	return l, nil
}

func (s *sqlMetaStore) GetLocationsByIDs(ctx context.Context, ids ...[]byte) (
	map[string]*pb.Location,
	[][]byte,
	error,
) {
	locs := map[string]*pb.Location{}

	mappingIDs := [][]byte{}
	mappingIDsSeen := map[string]struct{}{}

	remainingIds := []uuid.UUID{}
	for _, id := range ids {
		l, found, err := s.cache.getLocationByID(ctx, id)
		if err != nil {
			return nil, nil, fmt.Errorf("get location by ID: %w", err)
		}
		if found {
			locs[string(l.Id)] = l
			if len(l.MappingId) > 0 && !bytes.Equal(l.MappingId, uuid.Nil[:]) {
				if _, seen := mappingIDsSeen[string(l.MappingId)]; !seen {
					mappingIDs = append(mappingIDs, l.MappingId)
					mappingIDsSeen[string(l.MappingId)] = struct{}{}
				}
			}
			continue
		}

		lID, err := uuid.FromBytes(id)
		if err != nil {
			return nil, nil, fmt.Errorf("parse location id: %w", err)
		}

		remainingIds = append(remainingIds, lID)
	}

	if len(remainingIds) > 0 {
		dbctx, dbspan := s.tracer.Start(ctx, "GetLocationsByIDs-SQL-query")
		rows, err := s.db.QueryContext(dbctx, buildLocationsByIDsQuery(remainingIds))
		dbspan.End()
		if err != nil {
			return nil, nil, fmt.Errorf("execute SQL query: %w", err)
		}

		defer rows.Close()

		for rows.Next() {
			var (
				l         = &pb.Location{}
				locID     string
				address   int64
				mappingID *string
			)

			err := rows.Scan(&locID, &mappingID, &address, &l.IsFolded)
			if err != nil {
				return nil, nil, fmt.Errorf("scan row: %w", err)
			}
			lID, err := uuid.Parse(locID)
			if err != nil {
				return nil, nil, fmt.Errorf("parse location ID: %w", err)
			}
			l.Id = lID[:]

			if mappingID != nil {
				mappingUUID, err := uuid.Parse(*mappingID)
				if err != nil {
					return nil, nil, fmt.Errorf("parse location ID: %w", err)
				}

				l.MappingId = mappingUUID[:]
			}
			l.Address = uint64(address)
			if _, found := locs[string(l.Id)]; !found {
				err := s.cache.setLocationByID(ctx, l)
				if err != nil {
					return nil, nil, fmt.Errorf("set location cache by ID: %w", err)
				}
				locs[string(l.Id)] = l
				if mappingID != nil {
					if _, seen := mappingIDsSeen[string(l.MappingId)]; !seen {
						mappingIDs = append(mappingIDs, l.MappingId)
						mappingIDsSeen[string(l.MappingId)] = struct{}{}
					}
				}
			}
		}
		err = rows.Err()
		if err != nil {
			return nil, nil, fmt.Errorf("iterate over SQL rows: %w", err)
		}
	}

	return locs, mappingIDs, nil
}

const (
	locsByIDsQueryStart = `SELECT "id", "mapping_id", "address", "is_folded"
				FROM "locations"
				WHERE id IN (`
)

func buildLocationsByIDsQuery(ids []uuid.UUID) string {
	idLen := 36 // each serialized uuid is this length

	var totalLen int
	// Add the start of the query.
	totalLen += len(locsByIDsQueryStart)
	// The max value is known, and individual string can be larger than it.
	totalLen += len(ids) * idLen
	// len(ids)-1 commas, and a closing bracket is len(ids), plus two quotes per id.
	totalLen += 3 * len(ids)

	query := make([]byte, totalLen)
	copy(query, locsByIDsQueryStart)

	lastIndex := len(ids) - 1
	for i := range ids {
		var offset int
		// Add the start of the query.
		offset += len(locsByIDsQueryStart) - 1
		// The max value is known, and individual string can be larger than it.
		offset += i * idLen
		// len(ids)-1 commas, and a closing bracket is len(ids) plus 2 quotes surrounding each id.
		offset += 3 * i

		query[offset+1] = quote
		encodeID(query, offset+2, ids[i])
		query[offset+38] = quote
		if i < lastIndex {
			query[offset+39] = comma
		}
	}

	query[totalLen-1] = closingBracket
	return unsafeString(query)
}

func (s *sqlMetaStore) GetMappingsByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Mapping, error) {
	ctx, span := s.tracer.Start(ctx, "GetMappingsByIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("mapping-ids-length", len(ids)))

	res := make(map[string]*pb.Mapping, len(ids))

	sIds := ""
	for i, id := range ids {
		if i > 0 {
			sIds += ","
		}

		mUUID, err := uuid.FromBytes(id)
		if err != nil {
			return nil, err
		}

		sIds += "'" + mUUID.String() + "'"
	}

	query := fmt.Sprintf(
		`SELECT "id", "start", "limit", "offset", "file", "build_id",
				"has_functions", "has_filenames", "has_line_numbers", "has_inline_frames"
				FROM "mappings" WHERE id IN (%s)`, sIds)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("execute SQL query: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			m                    = &pb.Mapping{}
			id                   string
			start, limit, offset int64
		)
		err := rows.Scan(
			&id, &start, &limit, &offset, &m.File, &m.BuildId,
			&m.HasFunctions, &m.HasFilenames, &m.HasLineNumbers, &m.HasInlineFrames,
		)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrMappingNotFound
			}
			return nil, fmt.Errorf("scan row: %w", err)
		}
		mID, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("parse mapping ID: %w", err)
		}

		m.Id = mID[:]
		m.Start = uint64(start)
		m.Limit = uint64(limit)
		m.Offset = uint64(offset)

		if _, found := res[string(m.Id)]; !found {
			res[string(m.Id)] = m
		}
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate over SQL rows: %w", err)
	}

	return res, nil
}

func (s *sqlMetaStore) GetLinesByLocationIDs(ctx context.Context, ids ...[]byte) (map[string][]*pb.Line, [][]byte, error) {
	ctx, span := s.tracer.Start(ctx, "getLinesByLocationIDs")
	defer span.End()

	functionIDs := [][]byte{}
	functionIDsSeen := map[string]struct{}{}

	res := make(map[string][]*pb.Line, len(ids))
	remainingIds := []uuid.UUID{}
	for _, id := range ids {
		ll, found, err := s.cache.getLocationLinesByID(ctx, id)
		if err != nil {
			return res, functionIDs, fmt.Errorf("get location lines by ID from cache: %w", err)
		}
		if found {
			for _, l := range ll {
				if _, seen := functionIDsSeen[string(l.FunctionId)]; !seen {
					functionIDs = append(functionIDs, l.FunctionId)
					functionIDsSeen[string(l.FunctionId)] = struct{}{}
				}
			}
			res[string(id)] = ll
			continue
		}

		locUUID, err := uuid.FromBytes(id)
		if err != nil {
			return res, functionIDs, fmt.Errorf("parse location ID: %w", err)
		}

		remainingIds = append(remainingIds, locUUID)
	}

	if len(remainingIds) == 0 {
		return res, functionIDs, nil
	}

	rows, err := s.db.QueryContext(ctx, buildLinesByLocationIDsQuery(remainingIds))
	if err != nil {
		return nil, nil, fmt.Errorf("execute SQL query: %w", err)
	}

	defer rows.Close()

	retrievedLocationLines := make(map[string][]*pb.Line, len(ids))
	for rows.Next() {
		var (
			lID        string
			fID        string
			locationID uuid.UUID
			functionID uuid.UUID
			line       int64
		)
		l := &pb.Line{}
		err := rows.Scan(
			&lID, &l.Line, &fID,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("scan row:%w", err)
		}

		locationID, err = uuid.Parse(lID)
		if err != nil {
			return nil, nil, fmt.Errorf("parse function ID: %w", err)
		}

		functionID, err = uuid.Parse(fID)
		if err != nil {
			return nil, nil, fmt.Errorf("parse function ID: %w", err)
		}

		if _, found := retrievedLocationLines[string(locationID[:])]; !found {
			retrievedLocationLines[string(locationID[:])] = []*pb.Line{}
		}
		retrievedLocationLines[string(locationID[:])] = append(retrievedLocationLines[string(locationID[:])], &pb.Line{
			FunctionId: functionID[:],
			Line:       line,
		})

		if _, seen := functionIDsSeen[string(functionID[:])]; !seen {
			functionIDs = append(functionIDs, functionID[:])
			functionIDsSeen[string(functionID[:])] = struct{}{}
		}
	}
	err = rows.Err()
	if err != nil {
		return nil, nil, fmt.Errorf("iterate over SQL rows: %w", err)
	}

	for id, ll := range retrievedLocationLines {
		res[id] = ll
		err = s.cache.setLocationLinesByID(ctx, []byte(id), ll)
		if err != nil {
			return res, functionIDs, fmt.Errorf("set location lines by ID in cache: %w", err)
		}
	}

	return res, functionIDs, nil
}

const (
	linesByLocationsIDsQueryStart = `SELECT "location_id", "line", "function_id" FROM "lines" WHERE location_id IN (`
	comma                         = ','
	quote                         = '\''
	closingBracket                = ')'
)

func buildLinesByLocationIDsQuery(ids []uuid.UUID) string {
	idLen := 36 // Any uuid has this length as a string

	var totalLen int
	// Add the start of the query.
	totalLen += len(linesByLocationsIDsQueryStart)
	// The max value is known, and invididual string can be larger than it.
	totalLen += len(ids) * idLen
	// len(ids)-1 commas, and a closing bracket is len(ids) plus 2 quotes surrounding each id.
	totalLen += 3 * len(ids)

	query := make([]byte, totalLen)
	copy(query, linesByLocationsIDsQueryStart)

	lastIndex := len(ids) - 1
	for i := range ids {
		var offset int
		// Add the start of the query.
		offset += len(linesByLocationsIDsQueryStart) - 1
		// The max value is known, and individual string can be larger than it.
		offset += i * idLen
		// len(ids)-1 commas, and a closing bracket is len(ids) plus 2 quotes surrounding each id.
		offset += 3 * i

		query[offset+1] = quote
		encodeID(query, offset+2, ids[i])
		query[offset+38] = quote
		if i < lastIndex {
			query[offset+39] = comma
		}
	}

	query[totalLen-1] = closingBracket
	return unsafeString(query)
}

func encodeID(dst []byte, offset int, uuid uuid.UUID) {
	hex.Encode(dst[offset:], uuid[:4])
	dst[offset+8] = '-'
	hex.Encode(dst[offset+9:offset+13], uuid[4:6])
	dst[offset+13] = '-'
	hex.Encode(dst[offset+14:offset+18], uuid[6:8])
	dst[offset+18] = '-'
	hex.Encode(dst[offset+19:offset+23], uuid[8:10])
	dst[offset+23] = '-'
	hex.Encode(dst[offset+24:], uuid[10:])
}

func unsafeString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}

func (s *sqlMetaStore) GetFunctionsByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Function, error) {
	ctx, span := s.tracer.Start(ctx, "getFunctionsByIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("functions-ids-length", len(ids)))

	res := make(map[string]*pb.Function, len(ids))
	remainingIds := []uuid.UUID{}
	for _, id := range ids {
		f, found, err := s.cache.getFunctionByID(ctx, id)
		if err != nil {
			return res, fmt.Errorf("get function by ID from cache: %w", err)
		}
		if found {
			res[string(id)] = f
			continue
		}

		fuuid, err := uuid.FromBytes(id)
		if err != nil {
			return res, fmt.Errorf("parse function ID: %w", err)
		}

		remainingIds = append(remainingIds, fuuid)
	}

	if len(remainingIds) == 0 {
		return res, nil
	}

	sIds := ""
	for i, id := range remainingIds {
		if i > 0 {
			sIds += ","
		}
		sIds += "'" + id.String() + "'"
	}

	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(
			`SELECT "id", "name", "system_name", "filename", "start_line"
				FROM "functions" WHERE id IN (%s)`, sIds),
	)
	if err != nil {
		return nil, fmt.Errorf("execute SQL query: %w", err)
	}

	defer rows.Close()

	retrievedFunctions := make(map[string]*pb.Function, len(ids))
	for rows.Next() {
		var fIDString string
		f := &pb.Function{}
		err := rows.Scan(
			&fIDString, &f.Name, &f.SystemName, &f.Filename, &f.StartLine,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		fID, err := uuid.Parse(fIDString)
		if err != nil {
			return nil, fmt.Errorf("parse function ID: %w", err)
		}
		f.Id = fID[:]

		retrievedFunctions[string(f.Id)] = f
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}

	for id, f := range retrievedFunctions {
		res[id] = f
		err = s.cache.setFunctionByID(ctx, f)
		if err != nil {
			return res, fmt.Errorf("set function by ID in cache: %w", err)
		}
	}

	return res, nil
}

func (s *sqlMetaStore) CreateLocation(ctx context.Context, l *Location) ([]byte, error) {
	k := MakeSQLLocationKey(l)
	var (
		stmt *sql.Stmt
		err  error
		id   = uuid.New()
	)
	var f func() error
	if l.Mapping != nil {
		mID, err := uuid.FromBytes(l.Mapping.Id)
		if err != nil {
			return nil, fmt.Errorf("parse mapping ID: %w", err)
		}

		// Make sure mapping already exists in the database.
		_, err = s.getMappingByID(ctx, mID)
		if err != nil {
			return nil, fmt.Errorf("get mapping by id: %w", err)
		}

		stmt, err = s.db.PrepareContext(ctx, `INSERT INTO "locations" (
	                     id, address, is_folded, mapping_id, normalized_address, lines
	                     )
					values(?,?,?,?,?,?)`)
		if err != nil {
			return nil, fmt.Errorf("prepare SQL statement: %w", err)
		}
		defer stmt.Close()

		f = func() error {
			_, err = stmt.ExecContext(ctx, id.String(), int64(l.Address), l.IsFolded, mID.String(), int64(k.Address), k.Lines)
			return err
		}
	} else {
		stmt, err = s.db.PrepareContext(ctx, `INSERT INTO "locations" (
	                      id, address, is_folded, normalized_address, lines
	                     ) values(?,?,?,?,?)`)
		if err != nil {
			return nil, fmt.Errorf("CreateLocation failed: %w", err)
		}
		defer stmt.Close()

		f = func() error {
			_, err = stmt.ExecContext(ctx, id.String(), int64(l.Address), l.IsFolded, int64(k.Address), k.Lines)
			return err
		}
	}

	if err := backoff.Retry(f, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 3), ctx)); err != nil {
		return nil, fmt.Errorf("backoff SQL statement: %w", err)
	}

	if err != nil {
		return nil, fmt.Errorf("execute SQL statement: %w", err)
	}

	if err := s.CreateLocationLines(ctx, id[:], l.Lines); err != nil {
		return nil, fmt.Errorf("create lines: %w", err)
	}

	return id[:], nil
}

func (s *sqlMetaStore) Symbolize(ctx context.Context, l *Location) error {
	// NOTICE: We assume the given location is already persisted in the database.
	if err := s.CreateLocationLines(ctx, l.ID[:], l.Lines); err != nil {
		return fmt.Errorf("create lines: %w", err)
	}

	return nil
}

func (s *sqlMetaStore) GetLocations(ctx context.Context) ([]*pb.Location, [][]byte, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT l."id", l."address", l."is_folded", l."mapping_id"
				FROM "locations" l`,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("GetLocations failed: %w", err)
	}
	defer rows.Close()

	locs := []*pb.Location{}
	mappingIDsSeen := map[string]struct{}{}
	mappingIDs := [][]byte{}
	for rows.Next() {
		l := &pb.Location{}
		var (
			mappingID  *string
			locID      string
			locAddress int64
		)
		err := rows.Scan(
			&locID, &locAddress, &l.IsFolded, &mappingID,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("GetLocations failed: %w", err)
		}
		lUUID, err := uuid.Parse(locID)
		if err != nil {
			return nil, nil, fmt.Errorf("parse location ID: %w", err)
		}
		l.Id = lUUID[:]

		l.Address = uint64(locAddress)
		if mappingID != nil {
			id, err := uuid.Parse(*mappingID)
			if err != nil {
				return nil, nil, fmt.Errorf("parse mapping ID: %w", err)
			}

			if _, ok := mappingIDsSeen[string(id[:])]; !ok {
				mappingIDsSeen[string(id[:])] = struct{}{}
				mappingIDs = append(mappingIDs, id[:])
			}

			l.MappingId = id[:]
		}

		locs = append(locs, l)
	}
	return locs, mappingIDs, nil
}

func (s *sqlMetaStore) GetSymbolizableLocations(ctx context.Context) ([]*pb.Location, [][]byte, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT l."id", l."address", l."is_folded", l."mapping_id"
				FROM "locations" l
				LEFT JOIN "lines" ln ON l."id" = ln."location_id"
	            WHERE l.normalized_address > 0
	              AND ln."line" IS NULL
				  AND l."mapping_id" IS NOT NULL
	              AND l."id" IS NOT NULL`,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("GetSymbolizableLocations failed: %w", err)
	}
	defer rows.Close()

	locs := []*pb.Location{}
	mappingIDs := [][]byte{}
	mappingIDsSeen := map[string]struct{}{}
	for rows.Next() {
		l := &pb.Location{}
		var (
			mappingID  *string
			locID      string
			locAddress int64
		)
		err := rows.Scan(
			&locID, &locAddress, &l.IsFolded, &mappingID,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("GetSymbolizableLocations failed: %w", err)
		}

		id, err := uuid.Parse(locID)
		if err != nil {
			return nil, nil, fmt.Errorf("parse location ID: %w", err)
		}

		l.Id = id[:]
		l.Address = uint64(locAddress)
		if mappingID != nil {
			id, err := uuid.Parse(*mappingID)
			if err != nil {
				return nil, nil, fmt.Errorf("parse mapping ID: %w", err)
			}

			if _, ok := mappingIDsSeen[string(id[:])]; !ok {
				mappingIDs = append(mappingIDs, id[:])
				mappingIDsSeen[string(id[:])] = struct{}{}
			}

			l.MappingId = id[:]
		}

		locs = append(locs, l)
	}
	return locs, mappingIDs, nil
}

func (s *sqlMetaStore) GetFunctionByKey(ctx context.Context, fkey *pb.Function) (*pb.Function, error) {
	var id string

	k := MakeSQLFunctionKey(fkey)

	fn, found, err := s.cache.getFunctionByKey(ctx, k)
	if err != nil {
		return nil, fmt.Errorf("get function by key from cache: %w", err)
	}
	if found {
		return fn, nil
	}

	fn = &pb.Function{}

	if err := s.db.QueryRowContext(ctx,
		`SELECT "id", "name", "system_name", "filename", "start_line"
				FROM "functions"
				WHERE start_line=? AND name=? AND system_name=? AND filename=?`,
		k.StartLine, k.Name, k.SystemName, k.Filename,
	).Scan(&id, &fn.Name, &fn.SystemName, &fn.Filename, &fn.StartLine); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFunctionNotFound
		}
		return nil, fmt.Errorf("execute SQL statement: %w", err)
	}
	fnID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse function id: %w", err)
	}

	fn.Id = fnID[:]

	err = s.cache.setFunctionByKey(ctx, k, fn)
	if err != nil {
		return nil, fmt.Errorf("set function by key in cache: %w", err)
	}

	return fn, nil
}

func (s *sqlMetaStore) CreateFunction(ctx context.Context, fn *pb.Function) ([]byte, error) {
	var (
		stmt *sql.Stmt
		err  error
	)

	id := uuid.New()

	stmt, err = s.db.PrepareContext(ctx,
		`INSERT INTO "functions" (
	                     id, name, system_name, filename, start_line
	                     ) values(?,?,?,?,?)`,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateFunction failed: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, id.String(), fn.Name, fn.SystemName, fn.Filename, fn.StartLine)

	if err != nil {
		return nil, fmt.Errorf("CreateFunction failed: %w", err)
	}

	return id[:], nil
}

func (s *sqlMetaStore) GetFunctions(ctx context.Context) ([]*pb.Function, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT "id", "name", "system_name", "filename", "start_line" FROM "functions"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	funcs := []*pb.Function{}
	for rows.Next() {
		f := &pb.Function{}
		var id string
		err := rows.Scan(&id, &f.Name, &f.SystemName, &f.Filename, &f.StartLine)
		if err != nil {
			return nil, fmt.Errorf("GetFunctions failed: %w", err)
		}
		fID, err := uuid.Parse(id)
		if err != nil {
			return nil, err
		}
		f.Id = fID[:]

		funcs = append(funcs, f)
	}

	return funcs, nil
}

func (s *sqlMetaStore) GetMappingByKey(ctx context.Context, mkey *pb.Mapping) (*pb.Mapping, error) {
	var (
		start, limit, offset int64
		id                   string
	)

	k := MakeSQLMappingKey(mkey)

	m, found, err := s.cache.getMappingByKey(ctx, k)
	if err != nil {
		return nil, err
	}
	if found {
		return m, nil
	}

	m = &pb.Mapping{}

	if err := s.db.QueryRowContext(ctx,
		`SELECT "id", "start", "limit", "offset", "file", "build_id",
				"has_functions", "has_filenames", "has_line_numbers", "has_inline_frames"
				FROM "mappings"
				WHERE size=? AND offset=? AND build_id_or_file=?`,
		int64(k.Size), int64(k.Offset), k.BuildIDOrFile,
	).Scan(
		&id, &start, &limit, &offset, &m.File, &m.BuildId,
		&m.HasFunctions, &m.HasFilenames, &m.HasLineNumbers, &m.HasInlineFrames,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMappingNotFound
		}
		return nil, fmt.Errorf("GetMappingByKey failed: %w", err)
	}

	mID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse mapping ID: %w", err)
	}

	m.Id = mID[:]
	m.Start = uint64(start)
	m.Limit = uint64(limit)
	m.Offset = uint64(offset)

	err = s.cache.setMappingByKey(ctx, k, m)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (s *sqlMetaStore) CreateMapping(ctx context.Context, m *pb.Mapping) ([]byte, error) {
	var (
		stmt *sql.Stmt
		err  error
	)
	stmt, err = s.db.PrepareContext(ctx,
		`INSERT INTO "mappings" (
	                    "id", "start", "limit", "offset", "file", "build_id",
	                    "has_functions", "has_filenames", "has_line_numbers", "has_inline_frames",
	                    "size", "build_id_or_file"
	                    ) values(?,?,?,?,?,?,?,?,?,?,?,?)`,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateMapping failed: %w", err)
	}
	defer stmt.Close()

	k := MakeSQLMappingKey(m)
	id := uuid.New()
	_, err = stmt.ExecContext(ctx,
		id.String(), int64(m.Start), int64(m.Limit), int64(m.Offset), m.File, m.BuildId,
		m.HasFunctions, m.HasFilenames, m.HasLineNumbers, m.HasInlineFrames,
		int64(k.Size), k.BuildIDOrFile,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateMapping failed: %w", err)
	}

	return id[:], nil
}

func (s *sqlMetaStore) Close() error {
	return s.db.Close()
}

func (s *sqlMetaStore) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	return nil
}

func (s *sqlMetaStore) getMappingByID(ctx context.Context, mid uuid.UUID) (*pb.Mapping, error) {
	var (
		m                    *pb.Mapping
		start, limit, offset int64
		id                   uuid.UUID
	)

	m, found, err := s.cache.getMappingByID(ctx, mid[:])
	if err != nil {
		return nil, err
	}
	if found {
		return m, nil
	}

	m = &pb.Mapping{}

	err = s.db.QueryRowContext(ctx,
		`SELECT "id", "start", "limit", "offset", "file", "build_id",
				"has_functions", "has_filenames", "has_line_numbers", "has_inline_frames"
				FROM "mappings" WHERE id=?`, mid,
	).Scan(
		&id, &start, &limit, &offset, &m.File, &m.BuildId,
		&m.HasFunctions, &m.HasFilenames, &m.HasLineNumbers, &m.HasInlineFrames,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMappingNotFound
		}
		return nil, fmt.Errorf("getMappingByID failed: %w", err)
	}
	m.Id = id[:]
	m.Start = uint64(start)
	m.Limit = uint64(limit)
	m.Offset = uint64(offset)

	err = s.cache.setMappingByID(ctx, m)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (s *sqlMetaStore) getOrCreateFunction(ctx context.Context, f *pb.Function) ([]byte, error) {
	fn, err := s.GetFunctionByKey(ctx, f)
	if err == nil {
		return fn.Id, nil
	}
	if err != nil && err != ErrFunctionNotFound {
		return nil, err
	}

	return s.CreateFunction(ctx, f)
}

func (s *sqlMetaStore) CreateLocationLines(ctx context.Context, locID []byte, lines []LocationLine) error {
	if len(lines) > 0 {
		q := `INSERT INTO "lines" (location_id, line, function_id) VALUES `
		ll := make([]*pb.Line, 0, len(lines))
		var err error

		locUUID, err := uuid.FromBytes(locID)
		if err != nil {
			return fmt.Errorf("parse location ID: %w", err)
		}

		locUUIDString := locUUID.String()

		for i, ln := range lines {
			ln.Function.Id, err = s.getOrCreateFunction(ctx, ln.Function)
			if err != nil {
				return err
			}

			fID, err := uuid.FromBytes(ln.Function.Id)
			if err != nil {
				return fmt.Errorf("parse function ID: %w", err)
			}

			q += fmt.Sprintf(`('%s', %s, '%s')`,
				locUUIDString,
				strconv.FormatInt(ln.Line, 10),
				fID.String(),
			)
			if i != len(lines)-1 {
				q += ", "
			}
			ll = append(ll, &pb.Line{
				Line:       ln.Line,
				FunctionId: ln.Function.Id,
			})
		}
		q += ";"
		stmt, err := s.db.PrepareContext(ctx, q)
		if err != nil {
			return err
		}
		defer stmt.Close()

		_, err = stmt.ExecContext(ctx)
		if err != nil {
			return err
		}

		err = s.cache.setLocationLinesByID(ctx, locID, ll)
		if err != nil {
			return err
		}
	}
	return nil
}
