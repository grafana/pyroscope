package labels

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/dgraph-io/badger/v2"
)

type Labels struct {
	db   *badger.DB
	conn clickhouse.Conn
}

func New(db *badger.DB) *Labels {
	ll := &Labels{
		db: db,
	}
	return ll
}

func CHNew(conn clickhouse.Conn) *Labels {
	ll := &Labels{
		conn: conn,
	}
	return ll
}

func (ll *Labels) PutLabels(labels map[string]string) error {
	var queries []string
	ctx := context.Background()
	for k, v := range labels {
		queries = append(queries, fmt.Sprintf("INSERT INTO %s (key, value) VALUES ('l:%s', '')", "labels", k))
		queries = append(queries, fmt.Sprintf("INSERT INTO %s (key, value) VALUES ('v:%s:%s', '')", "labels", k, v))
	}
	query := strings.Join(queries, ";")

	err := ll.conn.Exec(ctx, query)
	if err != nil {
		// TODO: handle error
		panic(err)
	}
	return nil
}

func (ll *Labels) Put(key, val string) {

	ctx := context.Background()

	kk := "l:" + key
	kv := "v:" + key + ":" + val

	// Insert into "l" table
	query1 := fmt.Sprintf("INSERT INTO %s (key, value) VALUES ('%s', '')", "labels", kk)
	err := ll.conn.Exec(ctx, query1)
	if err != nil {
		// TODO: handle error
		panic(err)
	}

	fmt.Println("kv keval...", kv)

	// Insert into "v" table
	query2 := fmt.Sprintf("INSERT INTO %s (key, value) VALUES ('%s', '')", "labels", kv)
	err = ll.conn.Exec(ctx, query2)
	if err != nil {
		// TODO: handle error
		panic(err)
	} else {
		fmt.Println("also no error here ....")
	}
}

//revive:disable-next-line:get-return A callback is fine
func (ll *Labels) GetKeys(cb func(k string) bool) {
	ctx := context.Background()
	query := "SELECT DISTINCT substring(key, 3) AS k FROM labels WHERE key LIKE 'l:%'"
	rows, err := ll.conn.Query(ctx, query)
	if err != nil {
		// TODO: handle
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			// TODO: handle
			panic(err)
		}
		shouldContinue := cb(k)
		if !shouldContinue {
			return
		}
	}
	if err := rows.Err(); err != nil {
		// TODO: handle
		panic(err)
	}
}

// Delete removes key value label pair from the storage.
// If the pair can not be found, no error is returned.
func (ll *Labels) Delete(key, value string) error {
	kv := "v:" + key + ":" + value
	ctx := context.Background()
	query := fmt.Sprintf("ALTER TABLE %s DELETE WHERE key = ?", "labels")
	err := ll.conn.Exec(ctx, query, kv)
	return err
}

//revive:disable-next-line:get-return A callback is fine
func (ll *Labels) GetValues(key string, cb func(v string) bool) {
	ctx := context.Background()
	query := fmt.Sprintf("SELECT value FROM labels WHERE key = '%s'", key)
	rows, err := ll.conn.Query(ctx, query)
	if err != nil {
		// TODO: handle
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err != nil {
			// TODO: handle
			panic(err)
		}

		shouldContinue := cb(val)
		if !shouldContinue {
			return
		}
	}

	if err := rows.Err(); err != nil {
		// TODO: handle
		panic(err)
	}
}
