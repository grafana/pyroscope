package admin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
)

var (
	// ErrSocketStillResponding refers to when
	// a) an instance of the server is still running normally; or
	// b) server was not closed properly
	ErrSocketStillResponding = errors.New("a server is still running and responding to socket")
	// ErrInvalidSocketPathname refers to when the socket filepath is obviously invalid (eg empty string)
	ErrInvalidSocketPathname = errors.New("the socket filepath is invalid")
	// ErrListenerBind refers to generic errors
	ErrListenerBind = errors.New("could not listen on socket")

	// Anything works here
	SocketHTTPAddress = "http://pyroscope"
	HealthAddress     = SocketHTTPAddress + "/health"
)

type UdsHTTPServer struct {
	server     *http.Server
	listener   net.Listener
	socketAddr string
}

func NewUdsHTTPServer(socketAddr string) (*UdsHTTPServer, error) {
	if err := validateSocketAddress(socketAddr); err != nil {
		return nil, err
	}

	listener, err := createListener(socketAddr)
	if err != nil {
		return nil, err
	}

	return &UdsHTTPServer{
		listener:   listener,
		socketAddr: socketAddr,
	}, nil
}

func (u *UdsHTTPServer) Start(handler *http.ServeMux) error {
	// enrich with an additional endpoint
	// that we will use to probe when starting a new instance
	handler.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeMessage(w, 200, "it works!")
	})

	u.server = &http.Server{Handler: handler}

	return u.server.Serve(u.listener)
}

// createListener creates a listener on socketAddr UDS
// it tries to bind to socketAddr
// if it fails, it also tries to consume that socket
// if it's able to, it fails with ErrSocketStillResponding
// if it not able to, then it assumes it's a dangling socket and takes over it
//
// keep in mind there's a slight chance for a race condition there
// where a socket is verified to be not responding
// but the moment it's taken over, it starts to respond (probably because it was taken over by a different instance)
func createListener(socketAddr string) (net.Listener, error) {
	takeOver := func(socketAddr string) (net.Listener, error) {
		err := os.Remove(socketAddr)
		if err != nil {
			return nil, err
		}

		return net.Listen("unix", socketAddr)
	}

	// we listen on a unix domain socket
	// which will be created by the bind syscall
	// https://man7.org/linux/man-pages/man2/bind.2.html
	listener, err := net.Listen("unix", socketAddr)

	if err != nil {
		if isErrorAddressAlreadyInUse(err) {
			// that socket is already being used
			// let's check if the server is also responding
			httpClient, err := NewHTTPOverUDSClient(socketAddr)
			if err != nil {
				return nil, err
			}

			resp, err := httpClient.Get(HealthAddress)

			// the httpclient failed
			// let's take over
			// TODO identify what kind of error happened
			if err != nil {
				return takeOver(socketAddr)
			}

			// httpclient responded
			// let's check the status code
			if resp.StatusCode == http.StatusOK {
				return nil, ErrSocketStillResponding
			}

			// httpclient responded, but with a non 200 status code
			// let's be optimistic and try to take over
			return takeOver(socketAddr)
		}

		return nil, fmt.Errorf("could not bind to socket due to unrecoverable error: %w", err)
	}

	// no errors happened
	return listener, err
}

func (u *UdsHTTPServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err := u.server.Shutdown(ctx)
	if err != nil {
		return err
	}

	return os.Remove(u.socketAddr)
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

func validateSocketAddress(socketAddr string) error {
	if socketAddr == "" {
		return ErrInvalidSocketPathname
	}

	// TODO
	// check for the filepath size?
	// https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/sys_un.h.html#tag_13_67_04

	return nil
}
