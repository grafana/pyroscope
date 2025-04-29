package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-github/v58/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	mockclient "github.com/grafana/pyroscope/pkg/test/mocks/mockclient"
)

func TestGetCommit(t *testing.T) {
	tests := []struct {
		name          string
		mockCommit    *github.RepositoryCommit
		expected      *vcsv1.CommitInfo
		expectedError error
	}{
		{
			mockCommit: &github.RepositoryCommit{
				SHA: github.String("abc123"),
				Commit: &github.Commit{
					Message: github.String("test commit message"),
					Author: &github.CommitAuthor{
						Date: &github.Timestamp{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
				},
				Author: &github.User{
					Login:     github.String("test-user"),
					AvatarURL: github.String("https://example.com/avatar.png"),
				},
			},
			expected: &vcsv1.CommitInfo{
				Sha:     "abc123",
				Message: "test commit message",
				Author: &vcsv1.CommitAuthor{
					Login:     "test-user",
					AvatarURL: "https://example.com/avatar.png",
				},
				Date: "2024-01-01T00:00:00Z",
			},
		},
		{
			name: "example without author",
			mockCommit: &github.RepositoryCommit{
				SHA: github.String("abc123"),
				Commit: &github.Commit{
					Message: github.String("test commit message"),
					Author: &github.CommitAuthor{
						Date: &github.Timestamp{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
				},
			},
			expected: &vcsv1.CommitInfo{
				Sha:     "abc123",
				Message: "test commit message",
				Date:    "2024-01-01T00:00:00Z",
			},
		},
		{
			name: "fail without commit message",
			mockCommit: &github.RepositoryCommit{
				Commit: &github.Commit{
					Author: &github.CommitAuthor{
						Date: &github.Timestamp{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
				},
			},
			expectedError: connect.NewError(connect.CodeInternal, errors.New("commit contains no message")),
		},
		{
			name: "fail without commit date message",
			mockCommit: &github.RepositoryCommit{
				Commit: &github.Commit{
					Message: github.String("test commit message"),
				},
			},
			expectedError: connect.NewError(connect.CodeInternal, errors.New("commit contains no date")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepoService := &mockclient.MockrepositoryService{}
			// Create a mock GitHub client
			mockClient := &githubClient{
				repoService: mockRepoService,
			}
			mockRepoService.EXPECT().GetCommit(mock.Anything, "my-owner", "my-repo", "my-ref", mock.Anything).Return(tt.mockCommit, nil, nil)
			result, err := mockClient.GetCommit(context.Background(), "my-owner", "my-repo", "my-ref")

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.expectedError.Error())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
