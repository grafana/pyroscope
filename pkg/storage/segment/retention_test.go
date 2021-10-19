package segment

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/olekukonko/tablewriter"

	// "github.com/pyroscope-io/pyroscope/pkg/agent/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

/*

Given size limit L in bytes and threshold T (0;1).

Periodically check db size with `db.Size` call. Once size changes by L,
call `Flatten` and `GC` consequently: if value log files have been cleaned
up, trigger `Reclaim`. At this point size estimations (`item.EstimateSize`)
are quite precise and we can remove items from the database safely.

Reclaim ensures that db takes not more than (1-T)*L.

Notes/Questions:
 - db.Size is updated once per minute. It's better to measure size on owr own.
 - Count all key versions: the higher the estimation, the less data to be removed.
 - ValueLogFile should be adjusted depending on the L.

*/

//go:noinline
func work(n int) {
	// revive:disable:empty-block this is fine because this is a example app, not real production code
	for i := 0; i < n; i++ {
	}
	// revive:enable:empty-block
}

func TestX(t *testing.T) {
	const (
		dbPath       = "/Users/kolesnikovae/Documents/src/pyroscope/out/test_storage/trees"
		backupPath   = "/Users/kolesnikovae/Documents/src/pyroscope/out/trees_backup"
		restoredPath = "/Users/kolesnikovae/Documents/src/pyroscope/out/test_storage/trees_restored"
	)

	db := openDb(dbPath)
	measureDbSize("Open", db, dbPath, nil)

	measureDbSize("Flatten + GC", db, dbPath, func() {
		runGCWithFlatten(db)
	})

	measureDbSize("Remove keys (batch) + Flatten + GC", db, dbPath, func() {
		removeKeysBatch(db, 100, false)
		runGCWithFlatten(db) // Is it triggered?
	})

	measureDbSize("Backup", db, dbPath, func() {
		backup(db, backupPath)
	})

	closeDB(db)
	_ = os.RemoveAll(restoredPath)
	db = openDb(restoredPath)

	measureDbSize("Restore + Flatten + GC", db, restoredPath, func() {
		restore(db, backupPath)
		runGCWithFlatten(db)
	})

	closeDB(db)
}

func openDb(path string) *badger.DB {
	badgerOptions := badger.DefaultOptions(path).
		WithCompression(options.ZSTD).
		WithSyncWrites(false).       // true
		WithCompactL0OnClose(false). // true
		// WithNumVersionsToKeep(1).         // 1
		//	WithMaxTableSize(32 << 20).      // 64MB
		WithValueLogFileSize(64 << 20). // 1GB
		// WithNumLevelZeroTables(1).      // 5
		// WithNumLevelZeroTablesStall(2). // 15
		//	WithLevelOneSize(8 << 20)        // 256MB
		//	WithLevelSizeMultiplier(2)       // 10
		WithLoggingLevel(badger.DEBUG)

	db, err := badger.Open(badgerOptions)
	if err != nil {
		panic(err)
	}

	return db
}

func closeDB(db *badger.DB) {
	printTitle("Closing DB")
	if err := db.Close(); err != nil {
		panic(err)
	}
}

func runGCWithFlatten(db *badger.DB) {
	// Badger uses 2 compactors by default.
	if err := db.Flatten(2); err != nil {
		panic(err)
	}
	var (
		reclaimed bool
		err       error
	)
	for {
		// Does not have any impact?
		err = db.RunValueLogGC(0.5)
		if err == nil {
			reclaimed = true
			continue
		}
		break
	}
	if reclaimed {
		fmt.Println("Space reclaimed")
		return
	}
	fmt.Println(err)
}

func backup(db *badger.DB, path string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err = db.Backup(f, 0); err != nil {
		panic(err)
	}
}

func restore(db *badger.DB, path string) {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err = db.Load(f, 1<<10); err != nil {
		panic(err)
	}
}

func removeKeysBatch(db *badger.DB, n int, allVersions bool) {
	const batchSize = 128
	var (
		batch   = db.NewWriteBatch()
		deleted int
	)

	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: false,
			AllVersions:    allVersions,
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if deleted++; deleted >= n {
				return nil
			}
			if err := batch.Delete(item.KeyCopy(nil)); err != nil {
				return err
			}
			if deleted%batchSize == 0 {
				if err := batch.Flush(); err != nil {
					panic(err)
				}
				batch = db.NewWriteBatch()
			}
		}
		return nil
	})

	if err != nil {
		panic(err)
	}

	if deleted > 0 {
		if err = batch.Flush(); err != nil {
			panic(err)
		}
	}
}

func measureDbSize(name string, db *badger.DB, path string, f func()) {
	printTitle(name)
	if f != nil {
		f()
	}

	// DB size.
	printTable("Measured:", func(table *tablewriter.Table) {
		table.SetColumnSeparator(":")
		lsm, vlog := db.Size()
		table.Append([]string{"db.Size (LSM + VLOG)", bytesize.ByteSize(lsm + vlog).String()})
		table.Append([]string{"Size on disk", dirSize(path).String()})
	})

	// Estimated size.
	printTable("Estimated:", func(table *tablewriter.Table) {
		table.SetHeader([]string{
			"",
			"estimated size",
			"estimated deleted size",
			"keys",
			"deleted keys",
		})
		table.Append(dbSize(db, false).tableRow("last version"))
		table.Append(dbSize(db, true).tableRow("all versions"))
	})
}

func printTitle(s string) {
	fmt.Println()
	fmt.Println(strings.Repeat("–", 120))
	fmt.Println(strings.ToUpper(s))
	fmt.Println(strings.Repeat("- ", 60))
}

func printTable(name string, f func(table *tablewriter.Table)) {
	fmt.Println()
	fmt.Println(name)
	table := tablewriter.NewWriter(os.Stdout)
	table.SetBorder(false)
	f(table)
	table.Render()
}

type dbStats struct {
	keys        int
	size        int
	keysDeleted int
	sizeDeleted int
}

func (stats dbStats) tableRow(name string) []string {
	return []string{name,
		bytesize.ByteSize(stats.size).String(),
		bytesize.ByteSize(stats.sizeDeleted).String(),
		strconv.Itoa(stats.keys),
		strconv.Itoa(stats.keysDeleted),
	}
}

func dbSize(db *badger.DB, allVersions bool) (s dbStats) {
	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: false,
			AllVersions:    allVersions,
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if item.IsDeletedOrExpired() {
				s.keysDeleted++
				s.sizeDeleted += int(item.EstimatedSize())
				continue
			}
			s.keys++
			s.size += int(item.EstimatedSize())
		}
		return nil
	})

	if err != nil {
		panic(err)
	}

	return s
}

func dirSize(path string) (result bytesize.ByteSize) {
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			result += bytesize.ByteSize(info.Size())
		}
		return nil
	})
	return result
}
