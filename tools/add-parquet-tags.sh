#!/usr/bin/env bash

ROOT=$(git rev-parse --show-toplevel)

set -euo pipefail

set -x

# Ignore all fields on struct profile by default
gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct Profile -add-tags parquet -template "-" -w -quiet

# Profile
gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Profile -field TimeNanos -add-tags parquet -template "TimeNanos,delta" -w -quiet

for f in SampleType Sample Mapping Location Function StringTable; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct Profile -field "${f}" -add-tags parquet -template "${f}," -w -quiet -override
done

# SampleType
for f in Type Unit; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct ValueType -field "${f}" -add-tags parquet -template "${f}," -w -quiet -override
done

# Sample
for f in LocationId Value; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct Sample -field "${f}" -add-tags parquet -template "${f}," -w -quiet -override
done

# Label
gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Label -field Key -add-tags parquet -template "Key," -w -quiet
for f in Str NumUnit Num; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Label -field "${f}" -add-tags parquet -template "${f},optional" -w -quiet
done

# Symbol tables
for f in Id MemoryStart MemoryLimit FileOffset Filename BuildId HasFunctions HasFilenames HasLineNumbers HasInlineFrames; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Mapping -field "${f}" -add-tags parquet -template "${f}," -w -quiet
done

for f in Id MappingId Address Line IsFolded; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Location -field "${f}" -add-tags parquet -template "${f}," -w -quiet
done

for f in FunctionId Line; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Line -field "${f}" -add-tags parquet -template "${f}," -w -quiet
done

for f in Id Name SystemName Filename StartLine; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Function -field "${f}" -add-tags parquet -template "${f}," -w -quiet
done
