package github

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github-release-notifier/internal/domain"
)

const defaultBaseURL = "https://api.github.com"

type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	api        apiContract
}

type latestReleaseResponse struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

func NewClient(httpClient *http.Client, token string) *Client {
	return NewClientWithBaseURL(defaultBaseURL, httpClient, token)
}

func NewClientWithBaseURL(baseURL string, httpClient *http.Client, token string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		token:      token,
		api:        newDefaultAPIContract(),
	}
}

func (c *Client) RepositoryExists(ctx context.Context, owner string, repoName string) error {
	resp, err := c.doRequest(ctx, http.MethodGet, c.api.repositoryExistsURL(c.baseURL, owner, repoName))
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	return c.api.mapRepositoryExistsStatus(resp.StatusCode)
}

func (c *Client) GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.api.latestReleaseURL(c.baseURL, owner, repoName))
	if err != nil {
		return domain.Release{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if err := c.api.mapLatestReleaseStatus(resp.StatusCode); err != nil {
		return domain.Release{}, err
	}

	var payload latestReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return domain.Release{}, err
	}

	return domain.Release{
		TagName:     payload.TagName,
		Name:        payload.Name,
		HTMLURL:     payload.HTMLURL,
		Draft:       payload.Draft,
		Prerelease:  payload.Prerelease,
		PublishedAt: payload.PublishedAt,
	}, nil
}

func (c *Client) doRequest(ctx context.Context, method string, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}

	c.api.applyHeaders(req, c.token)

	return c.httpClient.Do(req)
}
