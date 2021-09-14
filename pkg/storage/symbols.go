package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/dgraph-io/badger/v2"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/profile"
)

type Symbols struct {
	d *dict.Dict
	// StackID -> dictionary key.
	m sync.Map
	// Dirty data to be flushed to disk.
	mu    sync.Mutex
	dirty []relation
}

type relation struct {
	stackID uint64
	dictKey []byte
}

func (s *Storage) Symbols(app string) *Symbols {
	// TODO: implement app unload
	if v, ok := s.symbols.Load(app); ok {
		return v.(*Symbols)
	}
	// TODO: avoid races Once per app.
	sym, err := s.loadSymbols(app)
	if err != nil {
		// TODO: error handling
		panic(err)
	}
	s.symbols.Store(app, sym)
	return sym
}

// TODO: refactor
func (s *Storage) loadSymbols(app string) (*Symbols, error) {
	sym := new(Symbols)
	var dictBuf []byte
	err := s.dbDicts.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("d:" + app))
		if err != nil {
			return err
		}
		dictBuf, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		prefix := []byte("m:" + app + ":")
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: true,
			PrefetchSize:   100,
			Prefix:         prefix,
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item = it.Item()
			buf, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			k, _ := binary.Uvarint(item.Key()[len(prefix):])
			sym.m.Store(k, buf)
		}

		return nil
	})

	switch {
	case err == nil:
	case errors.Is(err, badger.ErrKeyNotFound):
		return &Symbols{d: dict.New()}, nil
	default:
		return nil, err
	}
	sym.d, err = dict.FromBytes(dictBuf)
	if err != nil {
		return nil, err
	}

	return sym, nil
}

func (s *Storage) StoreSymbols() (err error) {
	s.symbols.Range(func(key, value interface{}) bool {
		v, ok := s.symbols.Load(key)
		if !ok {
			return true
		}
		sym := v.(*Symbols)
		sym.mu.Lock()
		defer sym.mu.Unlock()
		if len(sym.dirty) > 0 {
			err = s.storeSymbols(key.(string), sym)
		}
		return err == nil
	})
	return err
}

// TODO: refactor
func (s *Storage) storeSymbols(app string, sym *Symbols) error {
	buf, err := sym.d.Bytes()
	if err != nil {
		return err
	}
	prefix := "m:" + app + ":"
	keyBuf := make([]byte, binary.MaxVarintLen64)
	return s.dbDicts.Update(func(txn *badger.Txn) error {
		if err = txn.Set([]byte("d:"+app), buf); err != nil {
			return err
		}
		for _, r := range sym.dirty {
			n := binary.PutUvarint(keyBuf, r.stackID)
			k := append([]byte(prefix), keyBuf[:n]...)
			if err = txn.Set(k, r.dictKey); err != nil {
				return err
			}
		}
		sym.dirty = sym.dirty[:0]
		return nil
	})
}

func (s *Symbols) Insert(p *profile.Profile, k []byte, v uint64) {
	p.Stacks = append(p.Stacks, profile.Stack{
		ID:    s.putStackSymbols(k),
		Value: v,
	})
}

func (s *Symbols) Walk(p *profile.Profile, fn func(k []byte, v uint64) bool) {
	for _, x := range p.Stacks {
		k, ok := s.getStackSymbols(x.ID)
		if !ok {
			panic(fmt.Sprintf("can not lookup symbols for stack %d", x.ID))
		}
		if !fn(k, x.Value) {
			return
		}
	}
}

func (s *Symbols) putStackSymbols(v []byte) uint64 {
	// TODO: there should be a better way.
	//  Shouldn't we have a bloom filter?
	tk := s.d.Put(v)
	// Hash objects could be reused.
	h := fnv.New64a()
	_, _ = h.Write(tk)
	id := h.Sum64()
	if _, loaded := s.m.LoadOrStore(id, []byte(tk)); !loaded {
		s.mu.Lock()
		s.dirty = append(s.dirty, relation{id, tk})
		s.mu.Unlock()
	}
	return id
}

func (s *Symbols) getStackSymbols(id uint64) ([]byte, bool) {
	if tk, ok := s.m.Load(id); ok {
		return s.d.Get(tk.([]byte))
	}
	// Not found in map, db must've been corrupted.
	return nil, false
}
