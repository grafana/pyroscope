package compactionworker

/*
func TestCompactBlocks(t *testing.T) {
	worker, err := New(util.TestLogger(t), nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	worker.storage, _ = testutil.NewFilesystemBucket(t, ctx, "testdata")

	var blockMetas compactorv1.CompletedJob // same contract, can break in the future
	blockMetasData, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(blockMetasData, &blockMetas)
	require.NoError(t, err)

	compactedBlocks, err := worker.compactBlocks(ctx, "job-123", blockMetas.Blocks)
	require.NoError(t, err)

	require.Len(t, compactedBlocks, 1)

	_ = worker.storage.Delete(ctx, "blocks")
}
*/
