package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github-release-notifier/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestClient_GetLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/repos/gin-gonic/gin/releases/latest", r.URL.Path)
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		require.Equal(t, "2026-03-10", r.Header.Get("X-GitHub-Api-Version"))
		require.Equal(t, "github-release-notifier", r.Header.Get("User-Agent"))
		require.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name":"v1.11.0",
			"name":"v1.11.0",
			"html_url":"https://github.com/gin-gonic/gin/releases/tag/v1.11.0",
			"draft":false,
			"prerelease":false,
			"published_at":"2025-09-20T10:00:00Z"
		}`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "secret-token")

	release, err := client.GetLatestRelease(context.Background(), "gin-gonic", "gin")
	require.NoError(t, err)
	require.Equal(t, "v1.11.0", release.TagName)
	require.Equal(t, "v1.11.0", release.Name)
	require.Equal(t, "https://github.com/gin-gonic/gin/releases/tag/v1.11.0", release.HTMLURL)
	require.False(t, release.Draft)
	require.False(t, release.Prerelease)
	require.Equal(t, time.Date(2025, 9, 20, 10, 0, 0, 0, time.UTC), release.PublishedAt)
}

func TestClient_RepositoryExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/repos/gin-gonic/gin", r.URL.Path)
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		require.Equal(t, "2026-03-10", r.Header.Get("X-GitHub-Api-Version"))
		require.Equal(t, "github-release-notifier", r.Header.Get("User-Agent"))
		require.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	err := client.RepositoryExists(context.Background(), "gin-gonic", "gin")
	require.NoError(t, err)
}

func TestClient_RepositoryExists_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	err := client.RepositoryExists(context.Background(), "gin-gonic", "gin")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestClient_RepositoryExists_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	err := client.RepositoryExists(context.Background(), "gin-gonic", "gin")
	require.ErrorIs(t, err, domain.ErrRateLimited)
}

func TestClient_RepositoryExists_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	err := client.RepositoryExists(context.Background(), "gin-gonic", "gin")
	require.ErrorIs(t, err, domain.ErrRateLimited)
}

func TestClient_RepositoryExists_ReturnsUnexpectedStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	err := client.RepositoryExists(context.Background(), "gin-gonic", "gin")
	require.EqualError(t, err, "github repository request failed: status 500")
}

func TestClient_GetLatestRelease_NoReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	_, err := client.GetLatestRelease(context.Background(), "gin-gonic", "gin")
	require.ErrorIs(t, err, domain.ErrNoReleases)
}

func TestClient_GetLatestRelease_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	_, err := client.GetLatestRelease(context.Background(), "gin-gonic", "gin")
	require.ErrorIs(t, err, domain.ErrRateLimited)
}

func TestClient_GetLatestRelease_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	_, err := client.GetLatestRelease(context.Background(), "gin-gonic", "gin")
	require.ErrorIs(t, err, domain.ErrRateLimited)
}

func TestClient_GetLatestRelease_ReturnsUnexpectedStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	_, err := client.GetLatestRelease(context.Background(), "gin-gonic", "gin")
	require.EqualError(t, err, "github latest release request failed: status 500")
}

func TestClient_GetLatestRelease_ReturnsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL, server.Client(), "")

	_, err := client.GetLatestRelease(context.Background(), "gin-gonic", "gin")
	require.Error(t, err)
}

func TestNewClientWithBaseURL_TrimRightSlash(t *testing.T) {
	client := NewClientWithBaseURL("https://api.github.com/", nil, "")

	require.Equal(t, "https://api.github.com", client.baseURL)
	require.NotNil(t, client.httpClient)
	contract, ok := client.api.(defaultAPIContract)
	require.True(t, ok)
	require.Equal(t, "github-release-notifier", contract.userAgent)
	require.Equal(t, "2026-03-10", contract.apiVersion)
}
