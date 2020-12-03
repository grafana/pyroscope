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
		return errors.New("use of closed network connection")
	} else {
		fl.ch <- conn
		return nil
	}
}

func (fl *FakeListener) Accept() (net.Conn, error) {
	conn, more := <-fl.ch
	if more {
		return conn, nil
	} else {
		return nil, errors.New("use of closed network connection")
	}
}

func (fl *FakeListener) Close() error {
	fl.mutex.Lock()
	defer fl.mutex.Unlock()

	if fl.closed {
		return errors.New("use of closed network connection")
	} else {
		fl.closed = true
		close(fl.ch)
		return nil
	}
}

func (fl *FakeListener) Addr() net.Addr {
	a, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:1234")
	return a
}
