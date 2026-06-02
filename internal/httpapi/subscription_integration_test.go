//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appdb "github-release-notifier/internal/db"
	"github-release-notifier/internal/dbtest"
	"github-release-notifier/internal/domain"
	"github-release-notifier/internal/mail"
	"github-release-notifier/internal/service"
	"github-release-notifier/internal/storage"

	"github.com/gin-gonic/gin"
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

type testTable string

const (
	tableMailOutbox          testTable = "mail_outbox"
	tableSubscriptions       testTable = "subscriptions"
	tableTrackedRepositories testTable = "tracked_repositories"
	tableUsers               testTable = "users"
)

var rowCountQueryByTable = map[testTable]string{
	tableMailOutbox:          `select count(*) from mail_outbox`,
	tableSubscriptions:       `select count(*) from subscriptions`,
	tableTrackedRepositories: `select count(*) from tracked_repositories`,
	tableUsers:               `select count(*) from users`,
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
		router:       NewRouter(NewSubscriptionHandler(subscriptionService)),
		githubClient: githubClient,
	}
}

func TestSubscriptionAPI_SubscribeQueuesConfirmationEmail(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	subscribeResponse := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")
	require.Equal(t, http.StatusOK, subscribeResponse.Code)
	require.Empty(t, subscribeResponse.Body.String())

	requireTableRowCount(t, fixture.db, tableUsers, 1)
	requireTableRowCount(t, fixture.db, tableTrackedRepositories, 1)
	requireTableRowCount(t, fixture.db, tableSubscriptions, 1)
	requireTableRowCount(t, fixture.db, tableMailOutbox, 1)
	requireTrackedRepository(t, fixture.db, "gin-gonic", "gin", "v1.11.0")

	tokens := requireSubscriptionTokens(t, fixture.db, "test@example.com", "gin-gonic/gin")
	requireLatestOutboxEmail(t, fixture.db, outboxRow{
		RecipientEmail: "test@example.com",
		Subject:        "Confirm your GitHub release subscription",
		HTMLBody:       "gin-gonic/gin",
	})
	requireLatestOutboxEmailContains(t, fixture.db, "http://example.test/api/confirm/"+tokens.ConfirmationToken)
	requireLatestOutboxEmailContains(t, fixture.db, "http://example.test/api/unsubscribe/"+tokens.CancellationToken)
}

func TestSubscriptionAPI_SubscribeDuplicateDoesNotQueueSecondEmail(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	firstResponse := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")
	require.Equal(t, http.StatusOK, firstResponse.Code)

	secondResponse := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")
	require.Equal(t, http.StatusConflict, secondResponse.Code)
	require.JSONEq(t, `{"error":"resource already exists"}`, secondResponse.Body.String())

	requireTableRowCount(t, fixture.db, tableUsers, 1)
	requireTableRowCount(t, fixture.db, tableTrackedRepositories, 1)
	requireTableRowCount(t, fixture.db, tableSubscriptions, 1)
	requireTableRowCount(t, fixture.db, tableMailOutbox, 1)
}

func TestSubscriptionAPI_SubscribeGitHubErrorDoesNotPersistBusinessDataOrOutbox(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	fixture.githubClient.releaseErr = domain.ErrRateLimited

	response := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.JSONEq(t, `{"error":"github is temporarily unavailable, please try again later"}`, response.Body.String())
	requireBusinessTablesAreClear(t, fixture.db)
	requireTableIsClear(t, fixture.db, tableMailOutbox)
}

func TestSubscriptionAPI_SubscribeRepositoryNotFoundDoesNotPersistBusinessDataOrOutbox(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	fixture.githubClient.existsErr = domain.ErrNotFound

	response := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")

	require.Equal(t, http.StatusNotFound, response.Code)
	require.JSONEq(t, `{"error":"resource not found"}`, response.Body.String())
	requireBusinessTablesAreClear(t, fixture.db)
	requireTableIsClear(t, fixture.db, tableMailOutbox)
}

func TestSubscriptionAPI_SubscribeRepositoryExistsRateLimitedDoesNotPersistBusinessDataOrOutbox(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	fixture.githubClient.existsErr = domain.ErrRateLimited

	response := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.JSONEq(t, `{"error":"github is temporarily unavailable, please try again later"}`, response.Body.String())
	requireBusinessTablesAreClear(t, fixture.db)
	requireTableIsClear(t, fixture.db, tableMailOutbox)
}

func TestSubscriptionAPI_SubscribeRepositoryWithoutReleasesCreatesSubscriptionWithEmptyLastSeenTag(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	fixture.githubClient.releaseErr = domain.ErrNoReleases

	response := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")

	require.Equal(t, http.StatusOK, response.Code)
	requireTableRowCount(t, fixture.db, tableUsers, 1)
	requireTableRowCount(t, fixture.db, tableTrackedRepositories, 1)
	requireTableRowCount(t, fixture.db, tableSubscriptions, 1)
	requireTableRowCount(t, fixture.db, tableMailOutbox, 1)
	requireTrackedRepository(t, fixture.db, "gin-gonic", "gin", "")
}

func TestSubscriptionAPI_SubscribeIncorrectRepositoryFormatDoesNotCallGitHubOrPersistData(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	response := fixture.postSubscribe(t, "test@example.com", "gin-gonic")

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.JSONEq(t, `{"error":"incorrect repository format"}`, response.Body.String())
	require.Equal(t, 0, fixture.githubClient.repositoryExistsCalls)
	require.Equal(t, 0, fixture.githubClient.latestReleaseCalls)
	requireBusinessTablesAreClear(t, fixture.db)
	requireTableIsClear(t, fixture.db, tableMailOutbox)
}

func TestSubscriptionAPI_SubscribeOutboxFailureRollsBackBusinessData(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	requireOutboxTableBroken(t, fixture.db)

	response := fixture.postSubscribe(t, "test@example.com", "gin-gonic/gin")

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.JSONEq(t, `{"error":"internal server error"}`, response.Body.String())
	requireBusinessTablesAreClear(t, fixture.db)
}

func TestSubscriptionAPI_ListReturnsSubscriptions(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	createSubscription(t, fixture, "test@example.com", "gin-gonic/gin")

	listBeforeConfirm := fixture.get(t, "/api/subscriptions?email=test@example.com")
	require.Equal(t, http.StatusOK, listBeforeConfirm.Code)
	require.JSONEq(t, `[
		{
			"email": "test@example.com",
			"repo": "gin-gonic/gin",
			"confirmed": false,
			"last_seen_tag": "v1.11.0"
		}
	]`, listBeforeConfirm.Body.String())
}

func TestSubscriptionAPI_ListReturnsEmptyArrayWhenEmailHasNoSubscriptions(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	response := fixture.get(t, "/api/subscriptions?email=missing@example.com")

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `[]`, response.Body.String())
}

func TestSubscriptionAPI_ListRejectsMissingEmail(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)

	response := fixture.get(t, "/api/subscriptions")

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.JSONEq(t, `{"error":"email is required"}`, response.Body.String())
}

func TestSubscriptionAPI_ConfirmSubscription(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	tokens := createSubscription(t, fixture, "test@example.com", "gin-gonic/gin")

	confirmResponse := fixture.get(t, "/api/confirm/"+tokens.ConfirmationToken)
	require.Equal(t, http.StatusOK, confirmResponse.Code)
	requireSubscriptionConfirmed(t, fixture.db, tokens.ConfirmationToken)
}

func TestSubscriptionAPI_ConfirmAlreadyConfirmedSubscription(t *testing.T) {
	fixture := setupSubscriptionAPIIntegrationTest(t)
	tokens := createSubscription(t, fixture, "test@example.com", "gin-gonic/gin")

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
	tokens := createSubscription(t, fixture, "test@example.com", "gin-gonic/gin")

	unsubscribeResponse := fixture.get(t, "/api/unsubscribe/"+tokens.CancellationToken)
	require.Equal(t, http.StatusOK, unsubscribeResponse.Code)
	requireTableIsClear(t, fixture.db, tableSubscriptions)
	requireTableRowCount(t, fixture.db, tableMailOutbox, 1)

	listAfterUnsubscribe := fixture.get(t, "/api/subscriptions?email=test@example.com")
	require.Equal(t, http.StatusOK, listAfterUnsubscribe.Code)
	require.JSONEq(t, `[]`, listAfterUnsubscribe.Body.String())
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

func requireBusinessTablesAreClear(t *testing.T, db *sql.DB) {
	t.Helper()

	requireTableIsClear(t, db, tableUsers)
	requireTableIsClear(t, db, tableTrackedRepositories)
	requireTableIsClear(t, db, tableSubscriptions)
}

func requireTableIsClear(t *testing.T, db *sql.DB, table testTable) {
	t.Helper()

	requireTableRowCount(t, db, table, 0)
}

func requireTableRowCount(t *testing.T, db *sql.DB, table testTable, expectedCount int) {
	t.Helper()

	query, ok := rowCountQueryByTable[table]
	require.True(t, ok, "unknown table %q", table)

	var actualCount int
	err := db.QueryRowContext(context.Background(), query).Scan(&actualCount)
	require.NoError(t, err)
	require.Equal(t, expectedCount, actualCount, "unexpected row count for table %s", table)
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

func requireLatestOutboxEmail(t *testing.T, db *sql.DB, expected outboxRow) {
	t.Helper()

	actual := requireLatestOutboxEmailRow(t, db)
	require.Equal(t, expected.RecipientEmail, actual.RecipientEmail)
	require.Equal(t, expected.Subject, actual.Subject)
	require.Contains(t, actual.HTMLBody, expected.HTMLBody)
}

func requireLatestOutboxEmailContains(t *testing.T, db *sql.DB, expectedContent string) {
	t.Helper()

	actual := requireLatestOutboxEmailRow(t, db)
	require.Contains(t, actual.HTMLBody, expectedContent)
}

func requireLatestOutboxEmailRow(t *testing.T, db *sql.DB) outboxRow {
	t.Helper()

	var result outboxRow
	err := db.QueryRowContext(
		context.Background(),
		`
			select recipient_email, subject, html_body
			from mail_outbox
			order by id desc
			limit 1
		`,
	).Scan(&result.RecipientEmail, &result.Subject, &result.HTMLBody)
	require.NoError(t, err)
	return result
}

func requireOutboxTableBroken(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.ExecContext(context.Background(), `alter table mail_outbox rename to broken_mail_outbox`)
	require.NoError(t, err)
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
