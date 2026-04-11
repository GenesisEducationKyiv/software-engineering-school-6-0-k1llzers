package github

import (
	"context"
	"encoding/json"
	"fmt"
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
	userAgent  string
}

type latestReleaseResponse struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
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
		userAgent:  "github-release-notifier",
	}
}

func (c *Client) GetLatestRelease(ctx context.Context, owner string, repoName string) (domain.Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, owner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.Release{}, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Release{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return domain.Release{}, domain.ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return domain.Release{}, fmt.Errorf("github latest release request failed: status %d", resp.StatusCode)
	}

	var payload latestReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return domain.Release{}, err
	}

	return domain.Release{
		TagName:     payload.TagName,
		Name:        payload.Name,
		Draft:       payload.Draft,
		Prerelease:  payload.Prerelease,
		PublishedAt: payload.PublishedAt,
	}, nil
}
