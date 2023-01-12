#!/usr/bin/env bash

ROOT=$(git rev-parse --show-toplevel)

set -euo pipefail

set -x

# Ignore all fields on struct profile by default
gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct Profile -add-tags parquet -template "-" -w -quiet

# Profile
gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Profile -field TimeNanos -add-tags parquet -template ",delta" -w -quiet

for f in SampleType Sample Mapping Location Function StringTable; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct Profile -field "${f}" -add-tags parquet -template "," -w -quiet -override
done

# SampleType
for f in Type Unit; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct ValueType -field "${f}" -add-tags parquet -template "," -w -quiet -override
done

# Sample
for f in LocationId Value; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -struct Sample -field "${f}" -add-tags parquet -template "," -w -quiet -override
done

# Label
for f in Str NumUnit Num; do
  gomodifytags -file "${ROOT}/api/gen/proto/go/google/v1/profile.pb.go" -override -struct Label -field "${f}" -add-tags parquet -template ",optional" -w -quiet
done
