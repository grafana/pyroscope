package storage

// import (
// 	"context"
// 	"time"

// 	"github.com/dgraph-io/badger/v2"

// 	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
// )

// // Cleanup removes malformed data from the storage.
// func (s *Storage) Cleanup(ctx context.Context) error {
// 	select {
// 	case <-s.stop:
// 		return nil
// 	default:
// 		// This means than s.tasksWG.Wait has not been called yet
// 		// and it is safe to start the cleanup. There is a negligible
// 		// chance that tasksWG.Wait is called concurrently before we
// 		// continue.
// 	}
// 	s.tasksWG.Add(1)
// 	defer s.tasksWG.Done()
// 	start := time.Now()
// 	s.logger.Debug("cleanup started")
// 	defer func() {
// 		s.logger.WithField("duration", time.Since(start)).Debug("cleanup finished")
// 	}()
// 	return s.cleanupTreesDB(ctx)
// }

// func (s *Storage) cleanupTreesDB(ctx context.Context) (err error) {
// 	batch := s.trees.NewWriteBatch()
// 	defer func() {
// 		err = batch.Flush()
// 	}()
// 	return s.trees.Update(func(txn *badger.Txn) error {
// 		it := txn.NewIterator(badger.IteratorOptions{Prefix: treePrefix.bytes()})
// 		defer it.Close()
// 		var c int64
// 		for it.Rewind(); it.Valid(); it.Next() {
// 			select {
// 			default:
// 			case <-ctx.Done():
// 				return nil
// 			case <-s.stop:
// 				return nil
// 			}
// 			item := it.Item()
// 			if k, ok := treePrefix.trim(item.Key()); ok {
// 				if _, _, err = segment.ParseTreeKey(string(k)); err == nil {
// 					continue
// 				}
// 			}
// 			if c == s.trees.MaxBatchCount()+1 {
// 				if err = batch.Flush(); err != nil {
// 					return err
// 				}
// 				batch = s.trees.NewWriteBatch()
// 				c = 0
// 			}
// 			if err = batch.Delete(item.KeyCopy(nil)); err != nil {
// 				return err
// 			}
// 			c++
// 		}
// 		return nil
// 	})
// }

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

// Cleanup removes malformed data from the storage.
func (s *Storage) Cleanup(ctx context.Context) error {
	select {
	case <-s.stop:
		return nil
	default:
		// This means than s.tasksWG.Wait has not been called yet
		// and it is safe to start the cleanup. There is a negligible
		// chance that tasksWG.Wait is called concurrently before we
		// continue.
	}
	s.tasksWG.Add(1)
	defer s.tasksWG.Done()
	start := time.Now()
	s.logger.Debug("cleanup started")
	defer func() {
		s.logger.WithField("duration", time.Since(start)).Debug("cleanup finished")
	}()
	return s.cleanupTreesDB(ctx)
}

func (s *Storage) cleanupTreesDB(ctx context.Context) (err error) {
	query := fmt.Sprintf("SELECT name FROM batch WHERE database = 'default' AND name LIKE '%s%%'", treePrefix)

	rows, err := s.trees.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		select {
		default:
		case <-ctx.Done():
			return nil
		case <-s.stop:
			return nil
		}

		var name string
		err = rows.Scan(&name)
		if err != nil {
			return err
		}

		if k, ok := treePrefix.trim([]byte(name)); ok {
			if _, _, err = segment.ParseTreeKey(string(k)); err == nil {
				continue
			}
		}

		_, err = s.trees.Exec(fmt.Sprintf("ALTER TABLE %s.%s DELETE WHERE name='%s'", s.trees.DBInstance(), s.trees.Name(), strings.ReplaceAll(name, "'", "''")))
		if err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	return nil
}
