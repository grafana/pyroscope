package storage

import (
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v2"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/profile"
)

type Symbols struct{ *dict.Dict }

type relation struct {
	stackID uint64
	dictKey []byte
}

func (s *Storage) Symbols(app string) Symbols {
	// TODO: implement app unload
	if v, ok := s.symbols.Load(app); ok {
		return v.(Symbols)
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
func (s *Storage) loadSymbols(app string) (Symbols, error) {
	var sym Symbols
	var dictBuf []byte
	err := s.dbDicts.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("d:" + app))
		if err != nil {
			return err
		}
		dictBuf, err = item.ValueCopy(nil)
		return err
	})

	switch {
	case err == nil:
		sym.Dict, err = dict.FromBytes(dictBuf)
		return sym, err
	case errors.Is(err, badger.ErrKeyNotFound):
		return Symbols{Dict: dict.New()}, nil
	default:
		return sym, err
	}
}

func (s *Storage) StoreSymbols() (err error) {
	s.symbols.Range(func(key, value interface{}) bool {
		v, ok := s.symbols.Load(key)
		if !ok {
			return true
		}
		sym := v.(Symbols)
		sym.Lock()
		defer sym.Unlock()
		err = s.storeSymbols(key.(string), sym)
		return err == nil
	})
	return err
}

func (s *Storage) storeSymbols(app string, sym Symbols) error {
	buf, err := sym.Bytes()
	if err != nil {
		return err
	}
	return s.dbDicts.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("d:"+app), buf)
	})
}

func (s *Symbols) Insert(p *profile.Profile, k []byte, v uint64) {
	p.Stacks = append(p.Stacks, profile.Stack{
		ID:    s.Store(k),
		Value: v,
	})
}

func (s *Symbols) Walk(p *profile.Profile, fn func(k []byte, v uint64) bool) {
	for _, x := range p.Stacks {
		k, ok := s.Load(x.ID)
		if !ok {
			panic(fmt.Sprintf("can not lookup symbols for stack %d", x.ID))
		}
		if !fn(k, x.Value) {
			return
		}
	}
}
