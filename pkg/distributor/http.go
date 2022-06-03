package distributor

import (
	"net/http"
)

// PushHandler reads a gzip-compressed proto from the HTTP body.
func (d *Distributor) PushHandler(w http.ResponseWriter, r *http.Request) {
}
