//go:build unit

package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github-release-notifier/internal/domain"
	appmetrics "github-release-notifier/internal/metrics"
	"github-release-notifier/internal/readmodel"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionServiceStub struct {
	err                error
	email              string
	repositoryFullName string
	confirmationToken  string
	cancellationToken  string
	listResult         []readmodel.SubscriptionView
}

func (s *subscriptionServiceStub) Subscribe(ctx context.Context, email string, repositoryFullName string) error {
	s.email = email
	s.repositoryFullName = repositoryFullName
	if s.err != nil {
		return s.err
	}

	return nil
}

func (s *subscriptionServiceStub) ConfirmSubscription(_ context.Context, token string) error {
	s.confirmationToken = token
	if s.err != nil {
		return s.err
	}

	return nil
}

func (s *subscriptionServiceStub) CancelSubscription(_ context.Context, token string) error {
	s.cancellationToken = token
	if s.err != nil {
		return s.err
	}

	return nil
}

func (s *subscriptionServiceStub) ListSubscriptions(_ context.Context, email string) ([]readmodel.SubscriptionView, error) {
	s.email = email
	if s.err != nil {
		return nil, s.err
	}

	return s.listResult, nil
}

func TestSubscriptionHandler_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &subscriptionServiceStub{}
	router := NewRouter(NewSubscriptionHandler(service), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodPost, "/api/subscribe", bytes.NewBufferString(`{"email":"test@example.com","repo":"gin-gonic/gin"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "test@example.com", service.email)
	require.Equal(t, "gin-gonic/gin", service.repositoryFullName)
	require.Empty(t, recorder.Body.String())
}

func TestSubscriptionHandler_Create_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{}), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodPost, "/api/subscribe", bytes.NewBufferString(`{"email":"not-an-email"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, `{"error":"invalid request body"}`, recorder.Body.String())
}

func TestSubscriptionHandler_List(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &subscriptionServiceStub{
		listResult: []readmodel.SubscriptionView{
			{
				Email:       "test@example.com",
				Repo:        "gin-gonic/gin",
				Confirmed:   true,
				LastSeenTag: "v1.11.0",
			},
		},
	}
	router := NewRouter(NewSubscriptionHandler(service), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions?email=test@example.com", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "test@example.com", service.email)
	require.JSONEq(t, `[
		{
			"email":"test@example.com",
			"repo":"gin-gonic/gin",
			"confirmed":true,
			"last_seen_tag":"v1.11.0"
		}
	]`, recorder.Body.String())
}

func TestSubscriptionHandler_List_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{}), newTestMetrics(t))

	testCases := []struct {
		name string
		url  string
		body string
	}{
		{name: "missing email", url: "/api/subscriptions", body: `{"error":"email is required"}`},
		{name: "invalid email", url: "/api/subscriptions?email=invalid", body: `{"error":"invalid email"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.JSONEq(t, tc.body, recorder.Body.String())
		})
	}
}

func TestSubscriptionHandler_List_InternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{err: errors.New("boom")}), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions?email=test@example.com", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.JSONEq(t, `{"error":"internal server error"}`, recorder.Body.String())
}

func TestSubscriptionHandler_Create_MapsDomainErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
		body       string
	}{
		{name: "already exists", err: domain.ErrAlreadyExists, statusCode: http.StatusConflict},
		{name: "not found", err: domain.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "incorrect repository format", err: domain.ErrIncorrectRepositoryFormat, statusCode: http.StatusBadRequest},
		{name: "rate limited", err: domain.ErrRateLimited, statusCode: http.StatusServiceUnavailable, body: `{"error":"github is temporarily unavailable, please try again later"}`},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{err: tc.err}), newTestMetrics(t))

			req := httptest.NewRequest(http.MethodPost, "/api/subscribe", bytes.NewBufferString(`{"email":"test@example.com","repo":"gin-gonic/gin"}`))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.statusCode, recorder.Code)
			if tc.body != "" {
				require.JSONEq(t, tc.body, recorder.Body.String())
			}
		})
	}
}

func TestSubscriptionHandler_Confirm(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &subscriptionServiceStub{}
	router := NewRouter(NewSubscriptionHandler(service), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodGet, "/api/confirm/11111111-1111-1111-1111-111111111111", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", service.confirmationToken)
}

func TestSubscriptionHandler_Confirm_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{}), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodGet, "/api/confirm/invalid", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, `{"error":"invalid confirmation token"}`, recorder.Body.String())
}

func TestSubscriptionHandler_Confirm_MapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "invalid token", err: domain.ErrInvalidToken, statusCode: http.StatusBadRequest},
		{name: "not found", err: domain.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{err: tc.err}), newTestMetrics(t))

			req := httptest.NewRequest(http.MethodGet, "/api/confirm/11111111-1111-1111-1111-111111111111", nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.statusCode, recorder.Code)
		})
	}
}

func TestSubscriptionHandler_Cancel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &subscriptionServiceStub{}
	router := NewRouter(NewSubscriptionHandler(service), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodGet, "/api/unsubscribe/22222222-2222-2222-2222-222222222222", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "22222222-2222-2222-2222-222222222222", service.cancellationToken)
}

func TestSubscriptionHandler_Cancel_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{}), newTestMetrics(t))

	req := httptest.NewRequest(http.MethodGet, "/api/unsubscribe/invalid", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, `{"error":"invalid cancellation token"}`, recorder.Body.String())
}

func TestSubscriptionHandler_Cancel_MapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "not found", err: domain.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{err: tc.err}), newTestMetrics(t))

			req := httptest.NewRequest(http.MethodGet, "/api/unsubscribe/22222222-2222-2222-2222-222222222222", nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			require.Equal(t, tc.statusCode, recorder.Code)
		})
	}
}

func TestRouter_ExposesMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(NewSubscriptionHandler(&subscriptionServiceStub{}), newTestMetrics(t))

	apiReq := httptest.NewRequest(http.MethodGet, "/api/unsubscribe/invalid", nil)
	apiRecorder := httptest.NewRecorder()
	router.ServeHTTP(apiRecorder, apiReq)
	require.Equal(t, http.StatusBadRequest, apiRecorder.Code)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "github_release_notifier_http_requests_total")
}

func newTestMetrics(t *testing.T) *appmetrics.Metrics {
	t.Helper()

	metricSet, err := appmetrics.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, metricSet.Shutdown(context.Background()))
	})

	return metricSet
}
