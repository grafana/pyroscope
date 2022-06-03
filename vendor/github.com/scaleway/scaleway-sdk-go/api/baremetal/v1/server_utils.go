package baremetal

import (
	"time"

	"github.com/scaleway/scaleway-sdk-go/internal/async"
	"github.com/scaleway/scaleway-sdk-go/internal/errors"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const (
	defaultRetryInterval = 15 * time.Second
	defaultTimeout       = 2 * time.Hour
)

// WaitForServerRequest is used by WaitForServer method.
type WaitForServerRequest struct {
	ServerID      string
	Zone          scw.Zone
	Timeout       *time.Duration
	RetryInterval *time.Duration
}

// WaitForServer wait for the server to be in a "terminal state" before returning.
// This function can be used to wait for a server to be created.
func (s *API) WaitForServer(req *WaitForServerRequest, opts ...scw.RequestOption) (*Server, error) {
	timeout := defaultTimeout
	if req.Timeout != nil {
		timeout = *req.Timeout
	}
	retryInterval := defaultRetryInterval
	if req.RetryInterval != nil {
		retryInterval = *req.RetryInterval
	}

	terminalStatus := map[ServerStatus]struct{}{
		ServerStatusReady:   {},
		ServerStatusStopped: {},
		ServerStatusError:   {},
		ServerStatusLocked:  {},
		ServerStatusUnknown: {},
	}

	server, err := async.WaitSync(&async.WaitSyncConfig{
		Get: func() (interface{}, bool, error) {
			res, err := s.GetServer(&GetServerRequest{
				ServerID: req.ServerID,
				Zone:     req.Zone,
			}, opts...)
			if err != nil {
				return nil, false, err
			}

			_, isTerminal := terminalStatus[res.Status]
			return res, isTerminal, err
		},
		Timeout:          timeout,
		IntervalStrategy: async.LinearIntervalStrategy(retryInterval),
	})
	if err != nil {
		return nil, errors.Wrap(err, "waiting for server failed")
	}

	return server.(*Server), nil
}

// WaitForServerInstallRequest is used by WaitForServerInstall method.
type WaitForServerInstallRequest struct {
	ServerID      string
	Zone          scw.Zone
	Timeout       *time.Duration
	RetryInterval *time.Duration
}

// WaitForServerInstall wait for the server install to be in a
// "terminal state" before returning.
// This function can be used to wait for a server to be installed.
func (s *API) WaitForServerInstall(req *WaitForServerInstallRequest, opts ...scw.RequestOption) (*Server, error) {
	timeout := defaultTimeout
	if req.Timeout != nil {
		timeout = *req.Timeout
	}
	retryInterval := defaultRetryInterval
	if req.RetryInterval != nil {
		retryInterval = *req.RetryInterval
	}

	installTerminalStatus := map[ServerInstallStatus]struct{}{
		ServerInstallStatusCompleted: {},
		ServerInstallStatusError:     {},
		ServerInstallStatusUnknown:   {},
	}

	server, err := async.WaitSync(&async.WaitSyncConfig{
		Get: func() (interface{}, bool, error) {
			res, err := s.GetServer(&GetServerRequest{
				ServerID: req.ServerID,
				Zone:     req.Zone,
			}, opts...)
			if err != nil {
				return nil, false, err
			}

			if res.Install == nil {
				return nil, false, errors.New("server creation has not begun for server %s", req.ServerID)
			}

			_, isTerminal := installTerminalStatus[res.Install.Status]
			return res, isTerminal, err
		},
		Timeout:          timeout,
		IntervalStrategy: async.LinearIntervalStrategy(retryInterval),
	})
	if err != nil {
		return nil, errors.Wrap(err, "waiting for server installation failed")
	}

	return server.(*Server), nil
}

// GetServerOffer returns the offer of a baremetal server
func (s *API) GetServerOffer(server *Server) (*Offer, error) {
	offer, err := s.GetOffer(&GetOfferRequest{
		OfferID: server.OfferID,
		Zone:    server.Zone,
	})
	if err != nil {
		return nil, err
	}

	return offer, nil
}

type GetOfferByNameRequest struct {
	OfferName string
	Zone      scw.Zone
}

// GetOfferByName returns an offer from its commercial name
func (s *API) GetOfferByName(req *GetOfferByNameRequest) (*Offer, error) {
	res, err := s.ListOffers(&ListOffersRequest{
		Zone: req.Zone,
	}, scw.WithAllPages())
	if err != nil {
		return nil, err
	}

	for _, offer := range res.Offers {
		if req.OfferName == offer.Name {
			return offer, nil
		}
	}

	return nil, errors.New("could not find the offer ID from name %s", req.OfferName)
}
