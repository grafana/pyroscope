package history

import (
	"context"
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type QueryID string

type EntryType string

const (
	EntryTypeRender  EntryType = "render"
	EntryTypeMerge   EntryType = "merge"
	EntryTypeCompare EntryType = "compare"
	EntryTypeDiff    EntryType = "diff"
)

type Entry struct {
	ID               QueryID
	Type             EntryType
	URL              string
	Referrer         string
	Timestamp        time.Time
	Profiles         []string
	UserID           uint
	UserEmail        string
	OrganizationName string
	Successful       bool
	Cancelled        bool
}

func (in *Entry) PopulateFromRequest(req *http.Request) {
	in.URL = req.URL.String()
	in.Referrer = req.Header.Get("Referer")
	if u, ok := model.UserFromContext(req.Context()); ok {
		in.UserID = u.ID
		in.UserEmail = *u.Email
	}
}

type Manager interface {
	Add(ctx context.Context, entry *Entry) (QueryID, error)
	Get(ctx context.Context, id QueryID) (*Entry, error)
	List(ctx context.Context, cursor string) ([]*Entry, string, error)
}
