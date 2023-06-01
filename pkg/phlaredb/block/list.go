package block

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/oklog/ulid"
)

func ListBlock(path string, ulidMinTime time.Time) (map[ulid.ULID]*Meta, error) {
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
