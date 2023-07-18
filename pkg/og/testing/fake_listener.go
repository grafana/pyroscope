package testing

import (
	"errors"
	"net"
	"sync"
)

// FakeListener implements net.Listener and has an extra method (Provide) that allows
//   you to queue a connection to be consumed via Accept.
type FakeListener struct {
	ch     chan net.Conn
	closed bool
	mutex  *sync.Mutex
}

func NewFakeListener() *FakeListener {
	return &FakeListener{
		ch:    make(chan net.Conn),
		mutex: &sync.Mutex{},
	}
}

func (fl *FakeListener) Provide(conn net.Conn) error {
	fl.mutex.Lock()
	defer fl.mutex.Unlock()

	if fl.closed {
		return errors.New("connection closed")
	}

	fl.ch <- conn
	return nil
}

func (fl *FakeListener) Accept() (net.Conn, error) {
	conn, more := <-fl.ch
	if more {
		return conn, nil
	}

	return nil, errors.New("connection closed")
}

func (fl *FakeListener) Close() error {
	fl.mutex.Lock()
	defer fl.mutex.Unlock()

	if fl.closed {
		return errors.New("connection closed")
	}

	fl.closed = true
	close(fl.ch)
	return nil
}

func (*FakeListener) Addr() net.Addr {
	a, _ := net.ResolveTCPAddr("tcp", "fake-listener:1111")
	return a
}
