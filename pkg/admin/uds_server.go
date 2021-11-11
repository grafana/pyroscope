package admin

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
)

type UdsHTTPServer struct {
	server     *http.Server
	listener   net.Listener
	socketAddr string
}

func NewUdsHTTPServer(socketAddr string) (*UdsHTTPServer, error) {
	if socketAddr == "" {
		return nil, errors.New("socket address must be defined")
	}

	listener, err := createListener(socketAddr)
	if err != nil {
		return nil, fmt.Errorf("error while creating listener: %w", err)
	}

	return &UdsHTTPServer{
		listener:   listener,
		socketAddr: socketAddr,
	}, nil
}

func (u *UdsHTTPServer) Start(handler *http.ServeMux) error {
	handler.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeMessage(w, 200, "it works!")
	})

	u.server = &http.Server{Handler: handler}

	return u.server.Serve(u.listener)
}

func createListener(socketAddr string) (net.Listener, error) {
	takeOver := func(socketAddr string) (net.Listener, error) {
		err := os.Remove(socketAddr)
		if err != nil {
			return nil, err
		}

		return net.Listen("unix", socketAddr)
	}

	// we listen on a unix domain socket
	// which will be created by the os
	// https://man7.org/linux/man-pages/man2/bind.2.html
	listener, err := net.Listen("unix", socketAddr)

	if err != nil {
		if isErrorAddressAlreadyInUse(err) {
			// that socket is already being used

			// let's check if the server is also responding
			httpClient := newHttpClient(socketAddr)
			resp, err := httpClient.Get("http://pyroscope/health")
			// if not, then we take over
			// TODO identify what kind of error happened
			if err != nil {
				return takeOver(socketAddr)
			}

			// if yes, then we fail
			if resp.StatusCode == http.StatusOK {
				return nil, errors.New(fmt.Sprintf("a server is still running and responding to socket %s. please stop that server first", socketAddr))
			}

			// if any other kind of error, we also take over
			return takeOver(socketAddr)
		}

		return nil, fmt.Errorf("another error happened %w", err)
	}

	// no errors happened
	return listener, err
}

func (u *UdsHTTPServer) Stop() error {
	// TODO
	// drain these connections?
	err := u.server.Close()
	if err != nil {
		return err
	}

	return os.Remove(u.socketAddr)
}

func handleListenError(socketAddr string, err error) error {
	if isErrorAddressAlreadyInUse(err) {
		// that socket is already being used

		// let's check if the server is also responding
		httpClient := newHttpClient(socketAddr)
		resp, err := httpClient.Get("http://pyroscope/health")
		// if not, then we take over
		// TODO identify what kind of error happened
		if err != nil {
			return nil
		}

		// if yes, then we fail
		if resp.StatusCode == http.StatusOK {
			return errors.New(fmt.Sprintf("a server is still running and responding to socket %s. please stop that server first", socketAddr))
		}

		// if not, then we take over
		return nil
	}

	// it's some other error
	if err != nil {
		return fmt.Errorf("another error happened %w", err)
	}

	return nil
}

// https://stackoverflow.com/a/65865898
func isErrorAddressAlreadyInUse(err error) bool {
	var eOsSyscall *os.SyscallError
	if !errors.As(err, &eOsSyscall) {
		return false
	}
	var errErrno syscall.Errno // doesn't need a "*" (ptr) because it's already a ptr (uintptr)
	if !errors.As(eOsSyscall, &errErrno) {
		return false
	}
	if errErrno == syscall.EADDRINUSE {
		return true
	}

	return false
}
