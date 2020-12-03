package csock

import "net"

func NewTCPCSock(addrStr string, cb func(r *Request) *Response) (*CSock, error) {
	addr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		return nil, err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return NewCSock(listener, cb), nil
}
