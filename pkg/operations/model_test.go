package operations

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_readQuery(t *testing.T) {
	tests := []struct {
		name string
		args *http.Request
		want *blockQuery
	}{
		{
			name: "happy path",
			args: httptest.NewRequest("GET", "/test?queryFrom=now-2h&queryTo=now&includeDeleted=true", nil),
			want: &blockQuery{From: "now-2h", To: "now", IncludeDeleted: true, parsedFrom: time.Now().Add(-2 * time.Hour), parsedTo: time.Now()},
		},
		{
			name: "defaults",
			args: httptest.NewRequest("GET", "/test", nil),
			want: &blockQuery{From: "now-24h", To: "now", IncludeDeleted: false, parsedFrom: time.Now().Add(-24 * time.Hour), parsedTo: time.Now()},
		},
		{
			name: "absolute time format",
			args: httptest.NewRequest("GET", "/test?queryFrom=2006-01-02T15:04:05Z", nil),
			want: &blockQuery{From: "2006-01-02T15:04:05Z", To: "now", IncludeDeleted: false, parsedFrom: time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC), parsedTo: time.Now()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertQueriesEqual(t, tt.want, readQuery(tt.args))
		})
	}
}

func assertQueriesEqual(t *testing.T, a, b *blockQuery) {
	assert.Equal(t, a.From, b.From)
	assert.Equal(t, a.To, b.To)
	assert.Equal(t, a.IncludeDeleted, b.IncludeDeleted)

	assert.WithinDuration(t, a.parsedFrom, b.parsedFrom, 1*time.Second)
	assert.WithinDuration(t, a.parsedTo, b.parsedTo, 1*time.Second)
}

func Test_sortBlockGroups(t *testing.T) {
	tests := []struct {
		name  string
		input []*blockGroup
		want  []*blockGroup
	}{
		{
			name: "basic",
			input: []*blockGroup{
				{MinTime: time.Date(2020, 1, 1, 12, 45, 0, 9, time.UTC)},
				{MinTime: time.Date(2020, 1, 1, 12, 45, 0, 0, time.UTC)},
				{MinTime: time.Date(2020, 1, 2, 13, 45, 67, 0, time.UTC)},
				{MinTime: time.Date(2020, 1, 2, 13, 45, 0, 0, time.UTC)},
				{MinTime: time.Date(2020, 1, 1, 13, 45, 0, 0, time.UTC)},
			},
			want: []*blockGroup{
				{MinTime: time.Date(2020, 1, 2, 13, 45, 67, 0, time.UTC)},
				{MinTime: time.Date(2020, 1, 2, 13, 45, 0, 0, time.UTC)},
				{MinTime: time.Date(2020, 1, 1, 13, 45, 0, 0, time.UTC)},
				{MinTime: time.Date(2020, 1, 1, 12, 45, 0, 9, time.UTC)},
				{MinTime: time.Date(2020, 1, 1, 12, 45, 0, 0, time.UTC)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortBlockGroupsByMinTimeDec(tt.input)
			assert.Equal(t, tt.want, tt.input)
		})
	}
}

func Test_sortBlockDetails(t *testing.T) {
	tests := []struct {
		name  string
		input []*blockDetails
		want  []*blockDetails
	}{
		{
			name: "basic",
			input: []*blockDetails{
				{MinTime: "2020-01-02T15:04:05Z"},
				{MinTime: "2020-03-02T15:04:05Z"},
				{MinTime: "2020-03-03T15:04:05Z"},
				{MinTime: "2020-01-45T15:04:05Z"},
				{MinTime: "2020-01-02T15:04:55Z"},
			},
			want: []*blockDetails{
				{MinTime: "2020-03-03T15:04:05Z"},
				{MinTime: "2020-03-02T15:04:05Z"},
				{MinTime: "2020-01-45T15:04:05Z"},
				{MinTime: "2020-01-02T15:04:55Z"},
				{MinTime: "2020-01-02T15:04:05Z"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortBlockDetailsByMinTimeDec(tt.input)
			assert.Equal(t, tt.want, tt.input)
		})
	}
}
