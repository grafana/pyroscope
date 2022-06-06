// Copyright 2021-2022 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package connect

import (
	"net/http"
)

type nopSender struct {
	spec    Spec
	header  http.Header
	trailer http.Header
}

var _ Sender = (*nopSender)(nil)

func newNopSender(spec Spec, header, trailer http.Header) *nopSender {
	return &nopSender{
		spec:    spec,
		header:  header,
		trailer: trailer,
	}
}

func (n *nopSender) Header() http.Header {
	return n.header
}

func (n *nopSender) Trailer() (http.Header, bool) {
	return n.trailer, true
}

func (n *nopSender) Spec() Spec {
	return n.spec
}

func (n *nopSender) Send(_ any) error {
	return nil
}

func (n *nopSender) Close(_ error) error {
	return nil
}

type nopReceiver struct {
	spec    Spec
	header  http.Header
	trailer http.Header
}

var _ Receiver = (*nopReceiver)(nil)

func newNopReceiver(spec Spec, header, trailer http.Header) *nopReceiver {
	return &nopReceiver{
		spec:    spec,
		header:  header,
		trailer: trailer,
	}
}

func (n *nopReceiver) Spec() Spec {
	return n.spec
}

func (n *nopReceiver) Header() http.Header {
	return n.header
}

func (n *nopReceiver) Trailer() (http.Header, bool) {
	return n.trailer, true
}

func (n *nopReceiver) Receive(_ any) error {
	return nil
}

func (n *nopReceiver) Close() error {
	return nil
}
