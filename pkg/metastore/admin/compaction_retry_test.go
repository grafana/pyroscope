package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
)

func TestCompactionHandler_RetryButtons(t *testing.T) {
	for _, path := range []string{"/metastore-compaction", "/metastore-compaction?tenant=tenant-b"} {
		t.Run(path, func(t *testing.T) {
			now := time.Now()
			state := testCompactionState(now)
			live := state.CompactionJobs[3].CloneVT()
			live.Name = "live-exhausted"
			live.LeaseExpiresAt = now.Add(time.Hour).UnixNano()
			state.CompactionJobs = append(state.CompactionJobs, live)
			client := mockmetastorev1.NewMockCompactionServiceClient(t)
			client.EXPECT().GetCompactionState(mock.Anything, mock.Anything).Return(state, nil).Once()
			a := &Admin{compactionClient: client}
			w := httptest.NewRecorder()
			a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, w.Code)
			doc, err := goquery.NewDocumentFromReader(w.Body)
			require.NoError(t, err)
			forms := doc.Find(`form[method="post"]`)
			require.Equal(t, 1, forms.Length(), "only the exhausted job with an expired lease is retryable")
			require.Equal(t, state.CompactionJobs[3].Name, forms.Find(`input[name="job"]`).AttrOr("value", ""))
			require.Equal(t, "30", forms.Find(`input[name="token"]`).AttrOr("value", ""))
			require.Empty(t, forms.AttrOr("action", ""), "submit to the current cell and view")
		})
	}
}

func TestCompactionHandler_RetryPost(t *testing.T) {
	for _, tc := range []struct {
		name        string
		form        url.Values
		crossOrigin bool
		rpcError    error
		want        int
		call        bool
	}{
		{"accepted", url.Values{"job": {"job"}, "token": {"30"}}, false, nil, http.StatusSeeOther, true},
		{"stale", url.Values{"job": {"job"}, "token": {"30"}}, false, status.Error(codes.FailedPrecondition, "refresh before retrying"), http.StatusConflict, true},
		{"missing job", url.Values{"token": {"30"}}, false, nil, http.StatusBadRequest, false},
		{"bad token", url.Values{"job": {"job"}, "token": {"bad"}}, false, nil, http.StatusBadRequest, false},
		{"zero token", url.Values{"job": {"job"}, "token": {"0"}}, false, nil, http.StatusBadRequest, false},
		{"cross origin", url.Values{"job": {"job"}, "token": {"30"}}, true, nil, http.StatusForbidden, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := mockmetastorev1.NewMockCompactionServiceClient(t)
			if tc.call {
				client.EXPECT().RetryCompactionJob(mock.Anything, &metastorev1.RetryCompactionJobRequest{Name: "job", Token: 30}).Return(&metastorev1.RetryCompactionJobResponse{}, tc.rpcError).Once()
			}
			a := &Admin{compactionClient: client}
			a.compactionCache.ttl = time.Minute
			a.compactionCache.put("-", testCompactionState(time.Now()), time.Now())
			path := "/directory-phlare/profiles-prod-001/admin/metastore-compaction?tenant=tenant-b"
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.crossOrigin {
				r.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			w := httptest.NewRecorder()
			a.CompactionHandler().ServeHTTP(w, r)
			require.Equal(t, tc.want, w.Code)
			if tc.want == http.StatusSeeOther {
				location, err := url.Parse(w.Header().Get("Location"))
				require.NoError(t, err)
				require.Equal(t, path, r.URL.ResolveReference(location).RequestURI())
				require.Empty(t, a.compactionCache.entries)
			} else if tc.want == http.StatusConflict {
				require.Empty(t, a.compactionCache.entries)
				require.Equal(t, "Retry could not be confirmed. Refresh to check the job's current state.\n", w.Body.String())
				require.Empty(t, w.Header().Get("Location"))
			} else {
				require.Len(t, a.compactionCache.entries, 1)
			}
		})
	}
}
