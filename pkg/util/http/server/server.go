package server

import "net/http"

// EnableHTTP2 enables HTTP/1, TLS HTTP/2, and cleartext HTTP/2 on a server.
func EnableHTTP2(server *http.Server) {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	server.Protocols = protocols
}
