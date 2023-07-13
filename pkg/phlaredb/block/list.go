package block

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/thanos/pkg/block"
	"golang.org/x/sync/errgroup"

	phlareobj "github.com/grafana/phlare/pkg/objstore"
	"github.com/grafana/phlare/pkg/util"
)

func ListBlocks(path string, ulidMinTime time.Time) (map[ulid.ULID]*Meta, error) {
	result := make(map[ulid.ULID]*Meta)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, _, err := MetaFromDir(filepath.Join(path, entry.Name()))
		if err != nil {
			return nil, err
		}
		if !ulidMinTime.IsZero() && ulid.Time(meta.ULID.Time()).Before(ulidMinTime) {
			continue
		}
		result[meta.ULID] = meta
	}

	return result, nil
}

// IterBlockMetas iterates over all block metas in the given time range.
// It calls the given function for each block meta.
// It returns the first error returned by the function.
// It returns nil if all calls succeed.
// The function is called concurrently.
// Currently doesn't work with filesystem bucket.
func IterBlockMetas(ctx context.Context, bkt phlareobj.Bucket, from, to time.Time, fn func(*Meta)) error {
	allIDs, err := listAllBlockByPrefixes(ctx, bkt, from, to)
	if err != nil {
		return err
	}
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(128)

	// fetch all meta.json
	for _, ids := range allIDs {
		for _, id := range ids {
			id := id
			g.Go(func() error {
				r, err := bkt.Get(ctx, id+block.MetaFilename)
				if err != nil {
					return err
				}

				m, err := Read(r)
				if err != nil {
					return err
				}
				fn(m)
				return nil
			})
		}
	}
	return g.Wait()
}

func listAllBlockByPrefixes(ctx context.Context, bkt phlareobj.Bucket, from, to time.Time) ([][]string, error) {
	// todo: We should cache prefixes listing per tenants.
	blockPrefixes, err := blockPrefixesFromTo(from, to, 4)
	if err != nil {
		return nil, err
	}
	ids := make([][]string, len(blockPrefixes))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(64)

	for i, prefix := range blockPrefixes {
		prefix := prefix
		i := i
		g.Go(func() error {
			level.Debug(util.Logger).Log("msg", "listing blocks", "prefix", prefix, "i", i)
			prefixIds := []string{}
			err := bkt.Iter(ctx, prefix, func(name string) error {
				if _, ok := block.IsBlockDir(name); ok {
					prefixIds = append(prefixIds, name)
				}
				return nil
			}, objstore.WithoutApendingDirDelim)
			if err != nil {
				return err
			}
			ids[i] = prefixIds
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return ids, nil
}

// orderOfSplit is the number of bytes of the ulid id used for the split. The duration of the split is:
// 0: 1114y
// 1: 34.8y
// 2: 1y
// 3: 12.4d
// 4: 9h19m
// TODO: To needs to be adapted based on the MaxBlockDuration.
func blockPrefixesFromTo(from, to time.Time, orderOfSplit uint8) (prefixes []string, err error) {
	var id ulid.ULID

	if orderOfSplit > 9 {
		return nil, fmt.Errorf("order of split must be between 0 and 9")
	}

	byteShift := (9 - orderOfSplit) * 5

	ms := uint64(from.UnixMilli()) >> byteShift
	ms = ms << byteShift
	for ms <= uint64(to.UnixMilli()) {
		if err := id.SetTime(ms); err != nil {
			return nil, err
		}
		prefixes = append(prefixes, id.String()[:orderOfSplit+1])

		ms = ms >> byteShift
		ms += 1
		ms = ms << byteShift
	}

	return prefixes, nil
}

func SortBlocks(metas map[ulid.ULID]*Meta) []*Meta {
	var blocks []*Meta

	for _, b := range metas {
		blocks = append(blocks, b)
	}

	sort.Slice(blocks, func(i, j int) bool {
		// By min-time
		if blocks[i].MinTime != blocks[j].MinTime {
			return blocks[i].MinTime < blocks[j].MinTime
		}

		// Duration
		duri := blocks[i].MaxTime - blocks[i].MinTime
		durj := blocks[j].MaxTime - blocks[j].MinTime
		if duri != durj {
			return duri < durj
		}

		// ULID time.
		return blocks[i].ULID.Time() < blocks[j].ULID.Time()
	})
	return blocks
}
