package db

import (
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHealthTestStore creates a fresh store backed by a temp SQLite file.
func newHealthTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "health-test-*.db")
	require.NoError(t, err)
	_ = tmpFile.Close()
	store, err := NewStore(tmpFile.Name())
	require.NoError(t, err)
	return store, func() {
		_ = store.db.Close()
		_ = os.Remove(tmpFile.Name())
	}
}

func TestRecordFeedFailure_IncrementsAndStoresError(t *testing.T) {
	store, cleanup := newHealthTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	feedID, err := store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	at := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	fetchErr := errors.New("feed returned non-200 status code: 404")

	require.NoError(t, store.RecordFeedFailure(feedID, fetchErr, at))
	require.NoError(t, store.RecordFeedFailure(feedID, fetchErr, at.Add(time.Hour)))

	var (
		lastError           sql.NullString
		lastErrorAt         sql.NullTime
		consecutiveFailures int
		lastSuccessAt       sql.NullTime
	)
	require.NoError(t, store.db.QueryRow(
		`SELECT last_error, last_error_at, consecutive_failures, last_success_at FROM feeds WHERE id = ?`,
		feedID,
	).Scan(&lastError, &lastErrorAt, &consecutiveFailures, &lastSuccessAt))

	assert.Equal(t, 2, consecutiveFailures, "consecutive_failures should increment to 2")
	assert.True(t, lastError.Valid, "last_error should be set")
	assert.Contains(t, lastError.String, "404")
	assert.True(t, lastErrorAt.Valid, "last_error_at should be set")
	assert.Equal(t, at.Add(time.Hour), lastErrorAt.Time)
	assert.False(t, lastSuccessAt.Valid, "last_success_at should still be NULL after only failures")
}

func TestRecordFeedSuccess_ResetsFailureState(t *testing.T) {
	store, cleanup := newHealthTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	feedID, err := store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	// Seed a failure first.
	require.NoError(t, store.RecordFeedFailure(feedID, errors.New("boom"), time.Now()))

	at := time.Date(2026, 6, 20, 13, 0, 0, 0, time.UTC)
	require.NoError(t, store.RecordFeedSuccess(feedID, at))

	var (
		lastError           sql.NullString
		lastErrorAt         sql.NullTime
		consecutiveFailures int
		lastSuccessAt       sql.NullTime
	)
	require.NoError(t, store.db.QueryRow(
		`SELECT last_error, last_error_at, consecutive_failures, last_success_at FROM feeds WHERE id = ?`,
		feedID,
	).Scan(&lastError, &lastErrorAt, &consecutiveFailures, &lastSuccessAt))

	assert.Equal(t, 0, consecutiveFailures, "consecutive_failures should reset to 0")
	assert.False(t, lastError.Valid, "last_error should be cleared")
	assert.False(t, lastErrorAt.Valid, "last_error_at should be cleared")
	assert.True(t, lastSuccessAt.Valid, "last_success_at should be set")
	assert.Equal(t, at, lastSuccessAt.Time)
}

func TestGetUserFeeds_ReturnsHealthFields(t *testing.T) {
	store, cleanup := newHealthTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	feedID, err := store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	require.NoError(t, store.RecordFeedFailure(feedID, errors.New("dns error"), time.Now()))

	feeds, err := store.GetUserFeeds(userID)
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, 1, feeds[0].ConsecutiveFailures)
	assert.Equal(t, "dns error", feeds[0].LastError)
	assert.False(t, feeds[0].LastErrorAt.IsZero())
	assert.True(t, feeds[0].LastSuccessAt.IsZero(), "last_success_at should be zero when never succeeded")
}

func TestGetAllFeeds_ReturnsHealthFields(t *testing.T) {
	store, cleanup := newHealthTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	feedID, err := store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	at := time.Now().UTC()
	require.NoError(t, store.RecordFeedSuccess(feedID, at))

	feeds, err := store.GetAllFeeds()
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, 0, feeds[0].ConsecutiveFailures)
	assert.Equal(t, "", feeds[0].LastError)
	assert.True(t, feeds[0].LastSuccessAt.Equal(at), "last_success_at should match (got %v, want %v)", feeds[0].LastSuccessAt, at)
}

func TestGetFeedByURL_ReturnsHealthFields(t *testing.T) {
	store, cleanup := newHealthTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	feedURL := "https://example.com/feed.xml"
	feedID, err := store.AddFeedForUser(userID, feedURL)
	require.NoError(t, err)

	require.NoError(t, store.RecordFeedFailure(feedID, errors.New("timeout"), time.Now()))

	feed, err := store.GetFeedByURL(feedURL)
	require.NoError(t, err)
	require.NotNil(t, feed)
	assert.Equal(t, 1, feed.ConsecutiveFailures)
	assert.Equal(t, "timeout", feed.LastError)
}
