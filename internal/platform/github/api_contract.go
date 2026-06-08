package github

import (
	"fmt"
	"net/http"

	"github-release-notifier/internal/platform/rules"
	releasetracking "github-release-notifier/internal/release_tracking"
	"github-release-notifier/internal/shared"
)

type apiContract interface {
	repositoryExistsURL(baseURL string, owner string, repoName string) string
	latestReleaseURL(baseURL string, owner string, repoName string) string
	applyHeaders(req *http.Request, token string)
	mapRepositoryExistsStatus(statusCode int) error
	mapLatestReleaseStatus(statusCode int) error
}

type defaultAPIContract struct {
	userAgent  string
	apiVersion string

	repositoryExistsStatusMatcher rules.Matcher[int, error]
	latestReleaseStatusMatcher    rules.Matcher[int, error]
}

func newDefaultAPIContract() defaultAPIContract {
	return defaultAPIContract{
		userAgent:  "github-release-notifier",
		apiVersion: "2026-03-10",
		repositoryExistsStatusMatcher: newStatusMatcher(
			"github repository request failed",
			[]rules.Rule[int, error]{
				{Match: matchesStatus(http.StatusOK), Handle: noStatusError},
				{Match: matchesStatus(http.StatusNotFound), Handle: staticStatusError(shared.ErrNotFound)},
				{Match: matchesAnyStatus(http.StatusTooManyRequests, http.StatusForbidden), Handle: staticStatusError(shared.ErrRateLimited)},
			},
		),
		latestReleaseStatusMatcher: newStatusMatcher(
			"github latest release request failed",
			[]rules.Rule[int, error]{
				{Match: matchesStatus(http.StatusOK), Handle: noStatusError},
				{Match: matchesStatus(http.StatusNotFound), Handle: staticStatusError(releasetracking.ErrNoReleases)},
				{Match: matchesAnyStatus(http.StatusTooManyRequests, http.StatusForbidden), Handle: staticStatusError(shared.ErrRateLimited)},
			},
		),
	}
}

func (c defaultAPIContract) repositoryExistsURL(baseURL string, owner string, repoName string) string {
	return fmt.Sprintf("%s/repos/%s/%s", baseURL, owner, repoName)
}

func (c defaultAPIContract) latestReleaseURL(baseURL string, owner string, repoName string) string {
	return fmt.Sprintf("%s/repos/%s/%s/releases/latest", baseURL, owner, repoName)
}

func (c defaultAPIContract) applyHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", c.apiVersion)
	req.Header.Set("User-Agent", c.userAgent)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (c defaultAPIContract) mapRepositoryExistsStatus(statusCode int) error {
	return c.repositoryExistsStatusMatcher.Resolve(statusCode)
}

func (c defaultAPIContract) mapLatestReleaseStatus(statusCode int) error {
	return c.latestReleaseStatusMatcher.Resolve(statusCode)
}

func newStatusMatcher(defaultMessage string, ruleSet []rules.Rule[int, error]) rules.Matcher[int, error] {
	return rules.NewMatcher(ruleSet, func(statusCode int) error {
		return fmt.Errorf("%s: status %d", defaultMessage, statusCode)
	})
}

func matchesStatus(expected int) func(int) bool {
	return func(statusCode int) bool {
		return statusCode == expected
	}
}

func matchesAnyStatus(expected ...int) func(int) bool {
	return func(statusCode int) bool {
		for _, item := range expected {
			if statusCode == item {
				return true
			}
		}

		return false
	}
}

func staticStatusError(err error) func(int) error {
	return func(int) error {
		return err
	}
}

func noStatusError(int) error {
	return nil
}
