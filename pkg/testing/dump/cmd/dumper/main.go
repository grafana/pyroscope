package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
)

const usage = `Usage: dumper <flags> [command]

commands:
  show     Dumps raw key value to stdout
  list     Prints keys satisfying specified key prefix
  remove   Removes key from the database.

flags:
`

func main() {
	var (
		command string
		dbDir   string
		key     string
	)

	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
		fmt.Println()
		os.Exit(1)
	}

	flag.StringVar(&dbDir, "db-dir", "", "DB directory path")
	flag.StringVar(&key, "key", "", "key or prefix")
	flag.Parse()

	if len(flag.Args()) != 1 {
		flag.Usage()
	}

	if err := run(command, dbDir, []byte(key)); err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func run(command, dir string, key []byte) error {
	db, err := badger.Open(badger.DefaultOptions(dir).WithCompression(options.ZSTD))
	if err != nil {
		return err
	}
	switch command {
	case "list":
		return list(db, key)
	case "show":
		return show(db, key)
	case "remove":
		return remove(db, key)
	case "":
		flag.Usage()
		return fmt.Errorf("command is required")
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func list(db *badger.DB, prefix []byte) error {
	return db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			fmt.Printf("%s\n", it.Item().Key())
		}
		return nil
	})
}

func show(db *badger.DB, key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("key is required")
	}
	return db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		v, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		_, _ = io.Copy(os.Stdout, bytes.NewBuffer(v))
		return nil
	})
}

func remove(db *badger.DB, key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("key is required")
	}
	return db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}
