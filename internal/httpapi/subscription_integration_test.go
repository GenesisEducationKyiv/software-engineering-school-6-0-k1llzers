//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	appdb "github-release-notifier/internal/db"
	"github-release-notifier/internal/dbtest"
	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/mail"
	appmetrics "github-release-notifier/internal/metrics"
	"github-release-notifier/internal/service"
	"github-release-notifier/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type subscriptionAPIFixture struct {
	db           *sql.DB
	router       http.Handler
	githubClient *githubClientFake
}

type githubClientFake struct {
	existsErr             error
	release               domain.Release
	releaseErr            error
	repositoryExistsCalls int
	latestReleaseCalls    int
}

func (f *githubClientFake) RepositoryExists(_ context.Context, _ string, _ string) error {
	f.repositoryExistsCalls++
	return f.existsErr
}

func (f *githubClientFake) GetLatestRelease(_ context.Context, _ string, _ string) (domain.Release, error) {
	f.latestReleaseCalls++
	if f.releaseErr != nil {
		return domain.Release{}, f.releaseErr
	}

	return f.release, nil
}

type outboxRow struct {
	RecipientEmail string
	Subject        string
	HTMLBody       string
}

type subscriptionTokens struct {
	ConfirmationToken string
	CancellationToken string
}

type subscriptionTestData struct {
	Email string
	Owner string
	Name  string
	Repo  string
}

func setupSubscriptionAPIIntegrationTest(t *testing.T) subscriptionAPIFixture {
	t.Helper()

	gin.SetMode(gin.TestMode)

	_, db := dbtest.SetupTestPostgres(t)
	require.NoError(t, appdb.RunMigrations(context.Background(), db, filepath.Join("..", "..", "migrations")))

	renderer, err := mail.NewTemplateRenderer()
	require.NoError(t, err)

	githubClient := &githubClientFake{
		release: domain.Release{TagName: "v1.11.0"},
	}

	transactionManager := appdb.NewTransactionManager(db)
	userStore := storage.NewUserStore(db)
	trackedRepositoryStore := storage.NewTrackedRepositoryStore(db)
	subscriptionStore := storage.NewSubscriptionStore(db)
	outboxStore := storage.NewOutboxStore(db)
	mailService := mail.NewService(renderer, outboxStore, "http://example.test/api")
	subscriptionService := service.NewSubscriptionService(
		transactionManager,
		userStore,
		trackedRepositoryStore,
		subscriptionStore,
		githubClient,
		mailService,
	)

	return subscriptionAPIFixture{
		db:           db,
		router:       NewRouter(NewSubscriptionHandler(subscriptionService), newIntegrationTestMetrics(t)),
		githubClient: githubClient,
	}
}

func newSubscriptionTestData() subscriptionTestData {
	id := uuid.NewString()
	return subscriptionTestData{
		Email: fmt.Sprintf("test-%s@example.com", id),
		Owner: "gin",
		Name:  id,
		Repo:  "gin/" + id,
	}
}

func TestSubscriptionAPI_SubscribeQueuesConfirmationEmail(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()

	subscribeResponse := fixture.postSubscribe(t, data.Email, data.Repo)
	require.Equal(t, http.StatusOK, subscribeResponse.Code)
	require.Empty(t, subscribeResponse.Body.String())

	requireUserRowCount(t, fixture.db, data.Email, 1)
	requireTrackedRepositoryRowCount(t, fixture.db, data.Owner, data.Name, 1)
	requireSubscriptionRowCount(t, fixture.db, data.Email, data.Owner, data.Name, 1)
	requireOutboxEmailCount(t, fixture.db, data.Email, 1)
	requireTrackedRepository(t, fixture.db, data.Owner, data.Name, "v1.11.0")

	tokens := requireSubscriptionTokens(t, fixture.db, data.Email, data.Repo)
	requireOutboxEmail(t, fixture.db, data.Email, outboxRow{
		RecipientEmail: data.Email,
		Subject:        "Confirm your GitHub release subscription",
		HTMLBody:       data.Repo,
	})
	requireOutboxEmailContains(t, fixture.db, data.Email, "http://example.test/api/confirm/"+tokens.ConfirmationToken)
	requireOutboxEmailContains(t, fixture.db, data.Email, "http://example.test/api/unsubscribe/"+tokens.CancellationToken)
}

func TestSubscriptionAPI_SubscribeDuplicateDoesNotQueueSecondEmail(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()

	firstResponse := fixture.postSubscribe(t, data.Email, data.Repo)
	require.Equal(t, http.StatusOK, firstResponse.Code)

	secondResponse := fixture.postSubscribe(t, data.Email, data.Repo)
	require.Equal(t, http.StatusConflict, secondResponse.Code)
	require.JSONEq(t, `{"error":"resource already exists"}`, secondResponse.Body.String())

	requireUserRowCount(t, fixture.db, data.Email, 1)
	requireTrackedRepositoryRowCount(t, fixture.db, data.Owner, data.Name, 1)
	requireSubscriptionRowCount(t, fixture.db, data.Email, data.Owner, data.Name, 1)
	requireOutboxEmailCount(t, fixture.db, data.Email, 1)
}

func TestSubscriptionAPI_SubscribeGitHubErrorDoesNotPersistBusinessDataOrOutbox(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()
	fixture.githubClient.releaseErr = domain.ErrRateLimited

	response := fixture.postSubscribe(t, data.Email, data.Repo)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.JSONEq(t, `{"error":"github is temporarily unavailable, please try again later"}`, response.Body.String())
	requireNoBusinessDataOrOutbox(t, fixture.db, data)
}

func TestSubscriptionAPI_SubscribeRepositoryNotFoundDoesNotPersistBusinessDataOrOutbox(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()
	fixture.githubClient.existsErr = domain.ErrNotFound

	response := fixture.postSubscribe(t, data.Email, data.Repo)

	require.Equal(t, http.StatusNotFound, response.Code)
	require.JSONEq(t, `{"error":"resource not found"}`, response.Body.String())
	require.Equal(t, 1, fixture.githubClient.repositoryExistsCalls)
	require.Equal(t, 0, fixture.githubClient.latestReleaseCalls)
	requireNoBusinessDataOrOutbox(t, fixture.db, data)
}

func TestSubscriptionAPI_SubscribeRepositoryExistsRateLimitedDoesNotPersistBusinessDataOrOutbox(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()
	fixture.githubClient.existsErr = domain.ErrRateLimited

	response := fixture.postSubscribe(t, data.Email, data.Repo)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.JSONEq(t, `{"error":"github is temporarily unavailable, please try again later"}`, response.Body.String())
	require.Equal(t, 1, fixture.githubClient.repositoryExistsCalls)
	require.Equal(t, 0, fixture.githubClient.latestReleaseCalls)
	requireNoBusinessDataOrOutbox(t, fixture.db, data)
}

func TestSubscriptionAPI_SubscribeRepositoryWithoutReleasesCreatesSubscriptionWithEmptyLastSeenTag(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()
	fixture.githubClient.releaseErr = domain.ErrNoReleases

	response := fixture.postSubscribe(t, data.Email, data.Repo)

	require.Equal(t, http.StatusOK, response.Code)
	requireUserRowCount(t, fixture.db, data.Email, 1)
	requireTrackedRepositoryRowCount(t, fixture.db, data.Owner, data.Name, 1)
	requireSubscriptionRowCount(t, fixture.db, data.Email, data.Owner, data.Name, 1)
	requireOutboxEmailCount(t, fixture.db, data.Email, 1)
	requireTrackedRepository(t, fixture.db, data.Owner, data.Name, "")
}

func TestSubscriptionAPI_SubscribeIncorrectRepositoryFormatDoesNotCallGitHubOrPersistData(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()

	response := fixture.postSubscribe(t, data.Email, data.Owner)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.JSONEq(t, `{"error":"incorrect repository format"}`, response.Body.String())
	require.Equal(t, 0, fixture.githubClient.repositoryExistsCalls)
	require.Equal(t, 0, fixture.githubClient.latestReleaseCalls)
	requireNoBusinessDataOrOutbox(t, fixture.db, data)
}

func TestSubscriptionAPI_ListReturnsSubscriptions(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()

	createSubscription(t, fixture, data.Email, data.Repo)

	listBeforeConfirm := fixture.get(t, subscriptionListURL(data.Email))
	require.Equal(t, http.StatusOK, listBeforeConfirm.Code)
	requireSubscriptionsListResponse(t, listBeforeConfirm, []listSubscriptionsResponse{
		{
			Email:       data.Email,
			Repo:        data.Repo,
			Confirmed:   false,
			LastSeenTag: "v1.11.0",
		},
	})
}

func TestSubscriptionAPI_ListReturnsEmptyArrayWhenEmailHasNoSubscriptions(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()

	response := fixture.get(t, subscriptionListURL(data.Email))

	require.Equal(t, http.StatusOK, response.Code)
	requireSubscriptionsListResponse(t, response, nil)
}

func TestSubscriptionAPI_ListRejectsMissingEmail(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	response := fixture.get(t, "/api/subscriptions")

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.JSONEq(t, `{"error":"email is required"}`, response.Body.String())
}

func TestSubscriptionAPI_ConfirmSubscription(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()
	tokens := createSubscription(t, fixture, data.Email, data.Repo)

	confirmResponse := fixture.get(t, "/api/confirm/"+tokens.ConfirmationToken)
	require.Equal(t, http.StatusOK, confirmResponse.Code)
	requireSubscriptionConfirmed(t, fixture.db, tokens.ConfirmationToken)

	listAfterConfirm := fixture.get(t, subscriptionListURL(data.Email))
	require.Equal(t, http.StatusOK, listAfterConfirm.Code)
	requireSubscriptionsListResponse(t, listAfterConfirm, []listSubscriptionsResponse{
		{
			Email:       data.Email,
			Repo:        data.Repo,
			Confirmed:   true,
			LastSeenTag: "v1.11.0",
		},
	})
}

func TestSubscriptionAPI_ConfirmAlreadyConfirmedSubscription(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()
	tokens := createSubscription(t, fixture, data.Email, data.Repo)

	firstResponse := fixture.get(t, "/api/confirm/"+tokens.ConfirmationToken)
	require.Equal(t, http.StatusOK, firstResponse.Code)

	secondResponse := fixture.get(t, "/api/confirm/"+tokens.ConfirmationToken)
	require.Equal(t, http.StatusBadRequest, secondResponse.Code)
	require.JSONEq(t, `{"error":"invalid token"}`, secondResponse.Body.String())
	requireSubscriptionConfirmed(t, fixture.db, tokens.ConfirmationToken)
}

func TestSubscriptionAPI_ConfirmUnknownToken(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	response := fixture.get(t, "/api/confirm/11111111-1111-1111-1111-111111111111")

	require.Equal(t, http.StatusNotFound, response.Code)
	require.JSONEq(t, `{"error":"resource not found"}`, response.Body.String())
}

func TestSubscriptionAPI_UnsubscribeDeletesSubscription(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	data := newSubscriptionTestData()
	tokens := createSubscription(t, fixture, data.Email, data.Repo)

	unsubscribeResponse := fixture.get(t, "/api/unsubscribe/"+tokens.CancellationToken)
	require.Equal(t, http.StatusOK, unsubscribeResponse.Code)
	requireSubscriptionRowCount(t, fixture.db, data.Email, data.Owner, data.Name, 0)
	requireOutboxEmailCount(t, fixture.db, data.Email, 1)

	listAfterUnsubscribe := fixture.get(t, subscriptionListURL(data.Email))
	require.Equal(t, http.StatusOK, listAfterUnsubscribe.Code)
	requireSubscriptionsListResponse(t, listAfterUnsubscribe, nil)
}

func TestSubscriptionAPI_UnsubscribeUnknownToken(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	unsubscribeResponse := fixture.get(t, "/api/unsubscribe/22222222-2222-2222-2222-222222222222")
	require.Equal(t, http.StatusNotFound, unsubscribeResponse.Code)
	require.JSONEq(t, `{"error":"resource not found"}`, unsubscribeResponse.Body.String())
}

func (f subscriptionAPIFixture) postSubscribe(t *testing.T, email string, repo string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"email": email,
		"repo":  repo,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/subscribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return f.do(req)
}

func createSubscription(t *testing.T, fixture subscriptionAPIFixture, email string, repo string) subscriptionTokens {
	t.Helper()

	response := fixture.postSubscribe(t, email, repo)
	require.Equal(t, http.StatusOK, response.Code)
	return requireSubscriptionTokens(t, fixture.db, email, repo)
}

func (f subscriptionAPIFixture) get(t *testing.T, url string) *httptest.ResponseRecorder {
	t.Helper()

	return f.do(httptest.NewRequest(http.MethodGet, url, nil))
}

func (f subscriptionAPIFixture) do(req *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, req)
	return recorder
}

func subscriptionListURL(email string) string {
	return "/api/subscriptions?email=" + url.QueryEscape(email)
}

func requireNoBusinessDataOrOutbox(t *testing.T, db *sql.DB, data subscriptionTestData) {
	t.Helper()

	requireUserRowCount(t, db, data.Email, 0)
	requireTrackedRepositoryRowCount(t, db, data.Owner, data.Name, 0)
	requireSubscriptionRowCount(t, db, data.Email, data.Owner, data.Name, 0)
	requireOutboxEmailCount(t, db, data.Email, 0)
}

func requireUserRowCount(t *testing.T, db *sql.DB, email string, expectedCount int) {
	t.Helper()

	var actualCount int
	err := db.QueryRowContext(context.Background(), `select count(*) from users where email = $1`, email).Scan(&actualCount)
	require.NoError(t, err)
	require.Equal(t, expectedCount, actualCount, "unexpected user row count for email %s", email)
}

func requireTrackedRepositoryRowCount(t *testing.T, db *sql.DB, owner string, name string, expectedCount int) {
	t.Helper()

	var actualCount int
	err := db.QueryRowContext(
		context.Background(),
		`select count(*) from tracked_repositories where owner = $1 and name = $2`,
		owner,
		name,
	).Scan(&actualCount)
	require.NoError(t, err)
	require.Equal(t, expectedCount, actualCount, "unexpected tracked repository row count for %s/%s", owner, name)
}

func requireSubscriptionRowCount(t *testing.T, db *sql.DB, email string, owner string, name string, expectedCount int) {
	t.Helper()

	var actualCount int
	err := db.QueryRowContext(
		context.Background(),
		`
			select count(*)
			from subscriptions s
			join users u on u.id = s.user_id
			join tracked_repositories tr on tr.id = s.tracked_repository_id
			where u.email = $1 and tr.owner = $2 and tr.name = $3
		`,
		email,
		owner,
		name,
	).Scan(&actualCount)
	require.NoError(t, err)
	require.Equal(t, expectedCount, actualCount, "unexpected subscription row count for %s -> %s/%s", email, owner, name)
}

func requireOutboxEmailCount(t *testing.T, db *sql.DB, recipientEmail string, expectedCount int) {
	t.Helper()

	var actualCount int
	err := db.QueryRowContext(
		context.Background(),
		`select count(*) from mail_outbox where recipient_email = $1`,
		recipientEmail,
	).Scan(&actualCount)
	require.NoError(t, err)
	require.Equal(t, expectedCount, actualCount, "unexpected outbox row count for recipient %s", recipientEmail)
}

func requireTrackedRepository(t *testing.T, db *sql.DB, owner string, name string, lastSeenTag string) {
	t.Helper()

	var actualLastSeenTag string
	err := db.QueryRowContext(
		context.Background(),
		`select coalesce(last_seen_tag, '') from tracked_repositories where owner = $1 and name = $2`,
		owner,
		name,
	).Scan(&actualLastSeenTag)
	require.NoError(t, err)
	require.Equal(t, lastSeenTag, actualLastSeenTag)
}

func requireSubscriptionTokens(t *testing.T, db *sql.DB, email string, repo string) subscriptionTokens {
	t.Helper()

	var tokens subscriptionTokens
	err := db.QueryRowContext(
		context.Background(),
		`
			select s.confirmation_token::text, s.cancellation_token::text
			from subscriptions s
			join users u on u.id = s.user_id
			join tracked_repositories tr on tr.id = s.tracked_repository_id
			where u.email = $1 and tr.owner || '/' || tr.name = $2
		`,
		email,
		repo,
	).Scan(&tokens.ConfirmationToken, &tokens.CancellationToken)
	require.NoError(t, err)
	return tokens
}

func requireSubscriptionsListResponse(t *testing.T, response *httptest.ResponseRecorder, expected []listSubscriptionsResponse) {
	t.Helper()

	var actual []listSubscriptionsResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &actual))
	if expected == nil {
		require.Empty(t, actual)
		return
	}
	require.Equal(t, expected, actual)
}

func requireOutboxEmail(t *testing.T, db *sql.DB, recipientEmail string, expected outboxRow) {
	t.Helper()

	actual := requireOutboxEmailRow(t, db, recipientEmail)
	require.Equal(t, expected.RecipientEmail, actual.RecipientEmail)
	require.Equal(t, expected.Subject, actual.Subject)
	require.Contains(t, actual.HTMLBody, expected.HTMLBody)
}

func requireOutboxEmailContains(t *testing.T, db *sql.DB, recipientEmail string, expectedContent string) {
	t.Helper()

	actual := requireOutboxEmailRow(t, db, recipientEmail)
	require.Contains(t, actual.HTMLBody, expectedContent)
}

func requireOutboxEmailRow(t *testing.T, db *sql.DB, recipientEmail string) outboxRow {
	t.Helper()

	var result outboxRow
	err := db.QueryRowContext(
		context.Background(),
		`
			select recipient_email, subject, html_body
			from mail_outbox
			where recipient_email = $1
			order by id desc
			limit 1
		`,
		recipientEmail,
	).Scan(&result.RecipientEmail, &result.Subject, &result.HTMLBody)
	require.NoError(t, err)
	return result
}

func requireSubscriptionConfirmed(t *testing.T, db *sql.DB, confirmationToken string) {
	t.Helper()

	var confirmed bool
	err := db.QueryRowContext(
		context.Background(),
		`select confirmed from subscriptions where confirmation_token = $1`,
		confirmationToken,
	).Scan(&confirmed)
	require.NoError(t, err)
	require.True(t, confirmed)
}

func newIntegrationTestMetrics(t *testing.T) *appmetrics.Metrics {
	t.Helper()

	metricSet, err := appmetrics.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, metricSet.Shutdown(context.Background()))
	})

	return metricSet
}
