package vcs

import (
	"context"
	"fmt"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/sync/errgroup"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
)

const maxConcurrentRequests = 10

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
	sp, ctx := opentracing.StartSpanFromContext(ctx, "getCommits")
	defer sp.Finish()
	sp.SetTag("owner", owner)
	sp.SetTag("repo", repo)
	sp.SetTag("ref_count", len(refs))

	type result struct {
		commit *vcsv1.CommitInfo
		err    error
	}

	resultsCh := make(chan result, maxConcurrentRequests)
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentRequests)

	for _, ref := range refs {
		ref := ref
		g.Go(func() error {
			commit, err := tryGetCommit(ctx, client, owner, repo, ref)
			select {
			case resultsCh <- result{commit, err}:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
	}

	go func() {
		_ = g.Wait() // ignore errors since they're handled in the `resultsCh`.
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
	sp, ctx := opentracing.StartSpanFromContext(ctx, "tryGetCommit")
	defer sp.Finish()
	sp.SetTag("owner", owner)
	sp.SetTag("repo", repo)
	sp.SetTag("ref", ref)

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
