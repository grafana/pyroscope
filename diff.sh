set -ex

#GO=/home/korniltsev/github/go-linux-amd64-bootstrap/bin/go
GO=go

rm inlines-pgo.txt inlines-no-pgo.txt log-pgo.txt log-no-pgo.txt || true

go clean -cache

CGO_ENABLED=0 ${GO} build \
  -pgo=merged.pb.gz -tags "netgo embedassets" \
  -ldflags "-extldflags \"-static\""  "-gcflags" "all=-m=2  -d=pgodebug=3  -d=pgoinline=3" \
  ./cmd/pyroscope 2>&1 |  tee log-pgo.txt

go clean -cache

CGO_ENABLED=0 ${GO} build \
  -tags "netgo embedassets" \
  -ldflags "-extldflags \"-static\""  "-gcflags" "all=-m=2  -d=pgodebug=3  -d=pgoinline=3" \
  ./cmd/pyroscope 2>&1 |  tee log-no-pgo.txt


RE="hot-node enabled|inlining call"
cat log-pgo.txt| grep -iE "${RE}" > inlines-pgo.txt
cat log-no-pgo.txt| grep -iE "${RE}" > inlines-no-pgo.txt

diff inlines-no-pgo.txt inlines-pgo.txt

