package block_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

const (
	defaultValidateBenchBlockPath = "/var/folders/n_/n694y4s176x9v42kyj3rr_tr0000gn/T/opencode/01KVST78893AT2XMBK68S1YBKA-block.bin"
	validateBenchObjectPath       = "blocks/11/7874/01KVST78893AT2XMBK68S1YBKA/block.bin"
	validateBenchStartTime        = int64(1782201142079)
	validateBenchEndTime          = int64(1782215542079)
)

func BenchmarkVerifySymbolsInDataset_RealBlock(b *testing.B) {
	ctx := context.Background()
	blockPath := os.Getenv("PYROSCOPE_VALIDATE_BLOCK_PATH")
	if blockPath == "" {
		blockPath = defaultValidateBenchBlockPath
	}
	if _, err := os.Stat(blockPath); err != nil {
		b.Skipf("set PYROSCOPE_VALIDATE_BLOCK_PATH to a downloaded block.bin: %v", err)
	}

	root := b.TempDir()
	require.NoError(b, linkOrCopyFile(blockPath, filepath.Join(root, filepath.FromSlash(validateBenchObjectPath))))
	bucket, _ := testutil.NewFilesystemBucket(b, ctx, root)

	obj, err := block.NewObjectFromPath(ctx, bucket, validateBenchObjectPath, block.WithObjectMaxSizeLoadInMemory(0))
	require.NoError(b, err)
	md := obj.Metadata()

	req := block.SymbolBloomLookupRequest{
		SymbolName: "cloud.google.com/go/storage.(*httpReader).Read",
		MinTime:    validateBenchStartTime,
		MaxTime:    validateBenchEndTime,
	}
	candidates, err := block.LookupSymbolBloomCandidates(ctx, bucket, md, req, block.WithObjectMaxSizeLoadInMemory(0))
	require.NoError(b, err)
	require.NotEmpty(b, candidates.Candidates)
	candidate, ok := firstVerifiedCandidate(ctx, bucket, md, candidates.Candidates, req.SymbolName, req.MinTime, req.MaxTime)
	require.True(b, ok, "no exact match found for %q", req.SymbolName)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		found, err := block.VerifySymbolsInDataset(ctx, bucket, md, candidate.DatasetIndex, []string{req.SymbolName}, nil, req.MinTime, req.MaxTime, block.WithObjectMaxSizeLoadInMemory(0))
		require.NoError(b, err)
		if len(found[req.SymbolName]) == 0 {
			b.Fatalf("symbol %q not found", req.SymbolName)
		}
	}
}

func firstVerifiedCandidate(ctx context.Context, bucket objstore.Bucket, md *metastorev1.BlockMeta, candidates []block.SymbolBloomIndexRow, symbolName string, minTime, maxTime int64) (block.SymbolBloomIndexRow, bool) {
	for _, candidate := range candidates {
		found, err := block.VerifySymbolsInDataset(ctx, bucket, md, candidate.DatasetIndex, []string{symbolName}, nil, minTime, maxTime, block.WithObjectMaxSizeLoadInMemory(0))
		if err == nil && len(found[symbolName]) > 0 {
			return candidate, true
		}
	}
	return block.SymbolBloomIndexRow{}, false
}

func linkOrCopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil || errors.Is(err, os.ErrExist) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}
