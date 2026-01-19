// Copyright 2022-2025 The Parca Authors
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

package debuginfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	debuginfopb "buf.build/gen/go/parca-dev/parca/protocolbuffers/go/parca/debuginfo/v1alpha1"

	"github.com/go-kit/log"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	ErrMetadataShouldExist     = errors.New("debuginfo metadata should exist")
	ErrMetadataUnexpectedState = errors.New("debuginfo metadata state is unexpected")
	ErrMetadataNotFound        = errors.New("debuginfo metadata not found")
	ErrUploadMetadataNotFound  = errors.New("debuginfo upload metadata not found")
	ErrUploadIDMismatch        = errors.New("debuginfo upload id mismatch")
)

type ObjectStoreMetadata struct {
	logger log.Logger

	bucket objstore.Bucket
}

func NewObjectStoreMetadata(logger log.Logger, bucket objstore.Bucket) *ObjectStoreMetadata {
	return &ObjectStoreMetadata{logger: log.With(logger, "component", "debuginfo-metadata"), bucket: bucket}
}

func (m *ObjectStoreMetadata) SetQuality(ctx context.Context, buildID string, typ debuginfopb.DebuginfoType, quality *debuginfopb.DebuginfoQuality) error {
	dbginfo, err := m.Fetch(ctx, buildID, typ)
	if err != nil {
		return err
	}

	dbginfo.Quality = quality

	if err := m.write(ctx, dbginfo); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

func (m *ObjectStoreMetadata) MarkAsUploading(ctx context.Context, buildID, uploadID, hash string, typ debuginfopb.DebuginfoType, startedAt *timestamppb.Timestamp) error {
	return m.write(ctx, &debuginfopb.Debuginfo{
		BuildId: buildID,
		Source:  debuginfopb.Debuginfo_SOURCE_UPLOAD,
		Upload: &debuginfopb.DebuginfoUpload{
			Id:        uploadID,
			Hash:      hash,
			State:     debuginfopb.DebuginfoUpload_STATE_UPLOADING,
			StartedAt: startedAt,
		},
		Type: typ,
	})
}

func (m *ObjectStoreMetadata) MarkAsUploaded(ctx context.Context, buildID, uploadID string, typ debuginfopb.DebuginfoType, finishedAt *timestamppb.Timestamp) error {
	dbginfo, err := m.Fetch(ctx, buildID, typ)
	if err != nil {
		return err
	}

	if dbginfo.Upload == nil {
		return ErrUploadMetadataNotFound
	}

	if dbginfo.Upload.Id != uploadID {
		return ErrUploadIDMismatch
	}

	dbginfo.Upload.State = debuginfopb.DebuginfoUpload_STATE_UPLOADED
	dbginfo.Upload.FinishedAt = finishedAt

	return m.write(ctx, dbginfo)
}

//func (m *ObjectStoreMetadata) MarkAsPurged(ctx context.Context, buildID string, typ debuginfopb.DebuginfoType) error {
//	dbginfo, err := m.Fetch(ctx, buildID, typ)
//	if err != nil {
//		return err
//	}
//
//	if dbginfo.Upload == nil {
//		return ErrUploadMetadataNotFound
//	}
//
//	dbginfo.Upload.State = DebuginfoUpload_STATE_PURGED
//
//	return m.write(ctx, dbginfo)
//}

func (m *ObjectStoreMetadata) Fetch(ctx context.Context, buildID string, typ debuginfopb.DebuginfoType) (*debuginfopb.Debuginfo, error) {
	path := metadataObjectPath(buildID, typ)
	r, err := m.bucket.Get(ctx, path)
	if err != nil {
		if m.bucket.IsObjNotFoundErr(err) {
			return nil, ErrMetadataNotFound
		}
		return nil, fmt.Errorf("fetch debuginfo metadata from object storage: %w", err)
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read debuginfo metadata from object storage: %w", err)
	}

	dbginfo := &debuginfopb.Debuginfo{}
	if err := protojson.Unmarshal(content, dbginfo); err != nil {
		return nil, fmt.Errorf("unmarshal debuginfo metadata: %w", err)
	}
	return dbginfo, nil
}

func (m *ObjectStoreMetadata) write(ctx context.Context, dbginfo *debuginfopb.Debuginfo) error {
	if dbginfo.BuildId == "" {
		return errors.New("build id is required to wirte debuginfo metadata")
	}

	// Writing in multiline mode to make it easier to read for humans.
	debuginfoJSON, err := (protojson.MarshalOptions{Multiline: true}).Marshal(dbginfo)
	if err != nil {
		return err
	}

	r := bytes.NewReader(debuginfoJSON)
	if err := m.bucket.Upload(ctx, metadataObjectPath(dbginfo.BuildId, dbginfo.Type), r); err != nil {
		return fmt.Errorf("write debuginfo metadata to object storage: %w", err)
	}
	return nil
}

func metadataObjectPath(buildID string, typ debuginfopb.DebuginfoType) string {
	switch typ {
	case debuginfopb.DebuginfoType_DEBUGINFO_TYPE_EXECUTABLE:
		return path.Join(buildID, "executable.metadata")
	case debuginfopb.DebuginfoType_DEBUGINFO_TYPE_SOURCES:
		return path.Join(buildID, "sources.metadata")
	default:
		return path.Join(buildID, "metadata")
	}
}
