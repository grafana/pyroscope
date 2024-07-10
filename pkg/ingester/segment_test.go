package ingester

import (
	"fmt"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestReadSegment(t *testing.T) {
	t.Skip("skipping")
	s := "/home/korniltsev/p/pyroscope/data/segments/32/anon/01J2E88MFPVGEMEEW8N044EWCQ/"
	block, _ := os.ReadFile(s + "block.bin")
	tsdb, _ := os.ReadFile(s + "local/01J2E88MFQ7BAQVVPTCJ6ZTAS2/index.tsdb")
	profiles, _ := os.ReadFile(s + "local/01J2E88MFQ7BAQVVPTCJ6ZTAS2/profiles.parquet")
	symbols, _ := os.ReadFile(s + "local/01J2E88MFQ7BAQVVPTCJ6ZTAS2/symbols.symdb")
	metapb, _ := os.ReadFile(s + "meta.pb")
	meta := &metastorev1.BlockMeta{}
	err := meta.UnmarshalVT(metapb)
	require.NoError(t, err)

	fmt.Printf("%+v\n", meta)

	//  - 0: profiles.parquet
	//  - 1: index.tsdb
	//  - 2: symbols.symdb

	assert.Equal(t, 1, len(meta.TenantServices))
	ts := meta.TenantServices[0]
	actualBlock := block[ts.TableOfContents[0]:ts.TableOfContents[1]]
	actualTsdb := block[ts.TableOfContents[1]:ts.TableOfContents[2]]
	actualSymbols := block[ts.TableOfContents[2]:ts.Size]

	assert.Equal(t, profiles, actualBlock)
	assert.Equal(t, tsdb, actualTsdb)
	assert.Equal(t, symbols, actualSymbols)

}
