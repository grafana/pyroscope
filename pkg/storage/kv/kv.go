package kv

type Storage interface {
	Get(key []byte) ([]byte, error)
	Set(key []byte, value []byte) error
	Del(key []byte) error
	Close() error
}
