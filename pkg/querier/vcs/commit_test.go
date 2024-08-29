package vcs

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-github/v58/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
)

type gitHubCommitGetterMock struct {
	mock.Mock
}

func (m *gitHubCommitGetterMock) GetCommit(ctx context.Context, owner, repo, ref string) (*vcsv1.CommitInfo, error) {
	args := m.Called(ctx, owner, repo, ref)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*vcsv1.CommitInfo), args.Error(1)
}

func TestGetCommits(t *testing.T) {
	tests := []struct {
		name            string
		refs            []string
		mockSetup       func(*gitHubCommitGetterMock)
		expectedCommits int
		expectedErrors  int
		expectError     bool
	}{
		{
			name: "All commits succeed",
			refs: []string{"ref1", "ref2"},
			mockSetup: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&vcsv1.CommitInfo{}, nil)
			},
			expectedCommits: 2,
			expectedErrors:  0,
			expectError:     false,
		},
		{
			name: "Partial fetch commits success",
			refs: []string{"ref1", "ref2", "ref3"},
			mockSetup: func(m *gitHubCommitGetterMock) {
				// ref1 succeeds on first try
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "ref1").Return(&vcsv1.CommitInfo{}, nil)
				// ref2 fails on first try, succeeds with "heads/" prefix
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "ref2").Return(nil, errors.New("not found"))
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "heads/ref2").Return(&vcsv1.CommitInfo{}, nil)
				// ref3 fails on all attempts
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "ref3").Return(nil, errors.New("not found"))
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "heads/ref3").Return(nil, errors.New("not found"))
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "tags/ref3").Return(nil, errors.New("not found"))
			},
			expectedCommits: 2,
			expectedErrors:  1,
			expectError:     false,
		},
		{
			name: "All commits fail to fetch",
			refs: []string{"ref1", "ref2"},
			mockSetup: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("not found"))
			},
			expectedCommits: 0,
			expectedErrors:  2,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGetter := new(gitHubCommitGetterMock)
			tt.mockSetup(mockGetter)

			commits, failedFetches, err := getCommits(context.Background(), mockGetter, "owner", "repo", tt.refs)

			assert.Len(t, commits, tt.expectedCommits)
			assert.Len(t, failedFetches, tt.expectedErrors)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockGetter.AssertExpectations(t)
		})
	}
}

func TestTryGetCommit(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*gitHubCommitGetterMock)
		ref       string
		wantErr   bool
	}{
		{
			name: "Direct commit hash",
			setupMock: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&vcsv1.CommitInfo{}, nil)
			},
			ref:     "abcdef",
			wantErr: false,
		},
		{
			name: "Branch reference with heads prefix",
			setupMock: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "main").Return(nil, errors.New("not found"))
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "heads/main").Return(&vcsv1.CommitInfo{}, nil)
			},
			ref:     "main",
			wantErr: false,
		},
		{
			name: "Tag reference with tags prefix",
			setupMock: func(m *gitHubCommitGetterMock) {
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, assert.AnError).Times(2)
				m.On("GetCommit", mock.Anything, mock.Anything, mock.Anything, "tags/v1").Return(&vcsv1.CommitInfo{}, nil).Times(1)
			},
			ref:     "v1",
			wantErr: false,
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
			ref:     "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGetter := new(gitHubCommitGetterMock)
			tt.setupMock(mockGetter)

			commit, err := tryGetCommit(context.Background(), mockGetter, "owner", "repo", tt.ref)

			if tt.wantErr {
				assert.Error(t, err)
				var githubErr *github.ErrorResponse
				assert.True(t, errors.As(err, &githubErr), "Expected a GitHub error")
				assert.Nil(t, commit)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, commit)
			}

			mockGetter.AssertExpectations(t)
		})
	}
}
