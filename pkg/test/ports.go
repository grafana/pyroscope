package test

import "net"

// GetFreePorts returns a number of free local port for the tests to listen on. Note this will make sure the returned ports do not overlap, by stopping to listen once all ports are allocated
func GetFreePorts(len int) (ports []int, err error) {
	ports = make([]int, len)
	for i := 0; i < len; i++ {
		var a *net.TCPAddr
		if a, err = net.ResolveTCPAddr("tcp", "127.0.0.1:0"); err == nil {
			var l *net.TCPListener
			if l, err = net.ListenTCP("tcp", a); err != nil {
				return nil, err
			}
			defer l.Close()
			ports[i] = l.Addr().(*net.TCPAddr).Port
		}
	}
	return ports, nil
}
