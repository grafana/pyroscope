package history

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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
	Referer          string
	Timestamp        time.Time
	AppName          string
	StartTime        time.Time
	EndTime          time.Time
	Profiles         []string
	UserID           string
	UserEmail        string
	OrganizationName string
}

func (in *Entry) PopulateFromRequest(req *http.Request) {
	in.URL = req.URL.String()
	in.Referer = req.Header.Get("Referer")
	if u, ok := model.UserFromContext(req.Context()); ok {
		in.UserID = strconv.Itoa(int(u.ID))
		in.UserEmail = *u.Email
	}
}

type Manager interface {
	Add(ctx context.Context, entry *Entry) (QueryID, error)
	Get(ctx context.Context, id QueryID) (*Entry, error)
	List(ctx context.Context, cursor string) ([]*Entry, string, error)
}

func GenerateQueryID() QueryID {
	return QueryID(strings.ReplaceAll(uuid.New().String(), "-", ""))
}
