package vcs

import (
	"context"
	"fmt"
	"sync"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
)

type gitHubCommitGetter interface {
	GetCommit(context.Context, string, string, string) (*vcsv1.CommitInfo, error)
}

// getCommits fetches multiple commits in parallel for a given repository.
// It attempts to retrieve commits for all provided refs and returns:
// 1. Successfully fetched commits
// 2. Errors for failed fetches
// 3. An overall error if no commits were successfully fetched
// This function provides partial success behavior, returning any commits
// that were successfully fetched along with errors for those that failed.
func getCommits(ctx context.Context, client gitHubCommitGetter, owner, repo string, refs []string) ([]*vcsv1.CommitInfo, []error, error) {
	type result struct {
		commit *vcsv1.CommitInfo
		err    error
	}

	var wg sync.WaitGroup
	resultsCh := make(chan result, len(refs))

	for _, ref := range refs {
		wg.Add(1)
		go func(ref string) {
			defer wg.Done()
			commit, err := tryGetCommit(ctx, client, owner, repo, ref)
			resultsCh <- result{commit, err}
		}(ref)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var validCommits []*vcsv1.CommitInfo
	var failedFetches []error
	for r := range resultsCh {
		if r.err != nil {
			failedFetches = append(failedFetches, r.err)
		}
		if r.commit != nil {
			validCommits = append(validCommits, r.commit)
		}
	}

	if len(validCommits) == 0 && len(failedFetches) > 0 {
		return nil, failedFetches, fmt.Errorf("failed to fetch any commits")
	}

	return validCommits, failedFetches, nil
}

// tryGetCommit attempts to retrieve a commit using different ref formats (commit hash, branch, tag).
// It tries each format in order and returns the first successful result.
func tryGetCommit(ctx context.Context, client gitHubCommitGetter, owner, repo, ref string) (*vcsv1.CommitInfo, error) {
	refFormats := []string{
		ref,            // Try as a commit hash
		"heads/" + ref, // Try as a branch
		"tags/" + ref,  // Try as a tag
	}

	var lastErr error
	for _, format := range refFormats {
		commit, err := client.GetCommit(ctx, owner, repo, format)
		if err == nil {
			return commit, nil
		}

		lastErr = err
	}

	return nil, lastErr
}
