package vcs

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-kit/log"
	"github.com/google/go-github/v58/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
)

type gitHubCommitGetterMock struct {
	mock.Mock
}

func (m *gitHubCommitGetterMock) GetCommit(ctx context.Context, owner, repo, ref string) (*vcsv1.GetCommitResponse, error) {
	args := m.Called(ctx, owner, repo, ref)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*vcsv1.GetCommitResponse), args.Error(1)
}

func TestTryGetCommit(t *testing.T) {
	svc := Service{logger: log.NewNopLogger()}

	tests := []struct {
		name       string
		setupMock  func(*gitHubCommitGetterMock)
		ref        string
		wantCommit *vcsv1.GetCommitResponse
		wantErr    bool
	}{
		{
			name: "Direct commit hash",
			setupMock: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(&vcsv1.GetCommitResponse{Sha: "abc123"}, nil)
			},
			ref:        "",
			wantCommit: &vcsv1.GetCommitResponse{Sha: "abc123"},
			wantErr:    false,
		},
		{
			name: "Branch reference with 'heads/' prefix",
			setupMock: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, assert.AnError).Times(1)
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "heads/main").
					Return(&vcsv1.GetCommitResponse{Sha: "def456"}, nil).Times(1)
			},
			ref:        "main",
			wantCommit: &vcsv1.GetCommitResponse{Sha: "def456"},
			wantErr:    false,
		},
		{
			name: "Tag reference with 'tags/' prefix",
			setupMock: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, assert.AnError).Times(2)
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "tags/v1").
					Return(&vcsv1.GetCommitResponse{Sha: "def456"}, nil).Times(1)
			},
			ref:        "v1",
			wantCommit: &vcsv1.GetCommitResponse{Sha: "def456"},
			wantErr:    false,
		},
		{
			name: "GitHub API returns not found error",
			setupMock: func(m *gitHubCommitGetterMock) {
				notFoundErr := &github.ErrorResponse{
					Response: &http.Response{StatusCode: http.StatusNotFound},
				}
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, notFoundErr).Times(3)
			},
			ref:        "nonexistent",
			wantCommit: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGetter := new(gitHubCommitGetterMock)
			tt.setupMock(mockGetter)

			gotCommit, err := svc.tryGetCommit(context.Background(), mockGetter, "owner", "repo", tt.ref)

			if tt.wantErr {
				assert.Error(t, err)
				var githubErr *github.ErrorResponse
				assert.True(t, errors.As(err, &githubErr), "Expected a GitHub error")
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantCommit, gotCommit)
			mockGetter.AssertExpectations(t)
		})
	}
}
