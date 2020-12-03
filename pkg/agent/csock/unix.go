package csock

import (
	"net"
	"os"
)

func NewUnixCSock(path string, cb func(r *Request) *Response) (*CSock, error) {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}

	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}
	err = os.Chmod(path, os.ModePerm)
	if err != nil {
		return nil, err
	}

	return NewCSock(listener, cb), nil
}
