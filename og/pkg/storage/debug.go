package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/dgraph-io/badger/v2"
	"github.com/gorilla/mux"
)

func (s *Storage) DebugExport(w http.ResponseWriter, r *http.Request) {
	select {
	default:
	case <-s.stop:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	n := mux.Vars(r)["db"]
	var d BadgerDBWithCache
	switch n {
	case "segments":
		d = s.segments
	case "trees":
		d = s.trees
	case "dicts":
		d = s.dicts
	case "dimensions":
		d = s.dimensions
	default:
		// Note that export from main DB is not allowed.
		http.Error(w, fmt.Sprintf("database %q not found", n), http.StatusNotFound)
		return
	}

	// TODO(kolesnikovae): Refactor routes registration and use gorilla Queries.
	k, ok := r.URL.Query()["k"]
	if !ok || len(k) != 1 {
		http.Error(w, "query parameter 'k' is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	err := d.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(k[0]))
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			_, err = io.Copy(w, bytes.NewBuffer(v))
			return err
		})
	})

	switch {
	case err == nil:
	case errors.Is(err, badger.ErrKeyNotFound):
		http.Error(w, fmt.Sprintf("key %q not found in %s", k[0], n), http.StatusNotFound)
	default:
		http.Error(w, fmt.Sprintf("failed to export value for key %q: %v", k[0], err), http.StatusInternalServerError)
	}
}
