package feed

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aggregat4/rssgrid/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldBackOff(t *testing.T) {
	interval := 30 * time.Minute
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		feed     db.Feed
		expected bool
	}{
		{
			name:     "no failures never backs off",
			feed:     db.Feed{ConsecutiveFailures: 0},
			expected: false,
		},
		{
			name:     "few failures within grace threshold do not back off",
			feed:     db.Feed{ConsecutiveFailures: 3, LastErrorAt: now.Add(-interval)},
			expected: false,
		},
		{
			name:     "five failures retried too soon backs off",
			feed:     db.Feed{ConsecutiveFailures: 5, LastErrorAt: now.Add(-1 * time.Minute)},
			expected: true,
		},
		{
			name:     "five failures after backoff window elapses do not back off",
			feed:     db.Feed{ConsecutiveFailures: 5, LastErrorAt: now.Add(-17 * time.Hour)},
			expected: false,
		},
		{
			name:     "many failures cap backoff at 24h",
			feed:     db.Feed{ConsecutiveFailures: 50, LastErrorAt: now.Add(-23 * time.Hour)},
			expected: true,
		},
		{
			name:     "many failures past 24h window do not back off",
			feed:     db.Feed{ConsecutiveFailures: 50, LastErrorAt: now.Add(-25 * time.Hour)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, shouldBackOff(tt.feed, now, interval))
		})
	}
}

// stubFetcher is a controllable FeedFetcher for updater tests.
type stubFetcher struct {
	content *FeedContent
	err     error
	calls   int
}

func (s *stubFetcher) FetchFeed(_ context.Context, _ string) (*FeedContent, error) {
	s.calls++
	return s.content, s.err
}

func newUpdaterTestStore(t *testing.T) (*db.Store, func()) {
	t.Helper()
	tmp, err := createTempFile(t)
	require.NoError(t, err)
	store, err := db.NewStore(tmp)
	require.NoError(t, err)
	return store, func() {
		_ = store.Close()
		_ = removeFile(tmp)
	}
}

func createTempFile(t *testing.T) (string, error) {
	t.Helper()
	f, err := os.CreateTemp("", "updater-test-*.db")
	if err != nil {
		return "", err
	}
	name := f.Name()
	_ = f.Close()
	return name, nil
}

func removeFile(path string) error {
	return os.Remove(path)
}

func TestUpdateFeeds_RecordsFailureOnFetchError(t *testing.T) {
	store, cleanup := newUpdaterTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	_, err = store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	fetchErr := errors.New("connection refused")
	updater := NewUpdaterWithFetcher(store, 30*time.Minute, 100, &stubFetcher{err: fetchErr})

	require.NoError(t, updater.updateFeeds(context.Background()))

	feeds, err := store.GetAllFeeds()
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, 1, feeds[0].ConsecutiveFailures)
	assert.Equal(t, "connection refused", feeds[0].LastError)
	assert.False(t, feeds[0].LastErrorAt.IsZero())
}

func TestUpdateFeeds_RecordsSuccessOnContent(t *testing.T) {
	store, cleanup := newUpdaterTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	_, err = store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	content := &FeedContent{Title: "Test Feed"}
	updater := NewUpdaterWithFetcher(store, 30*time.Minute, 100, &stubFetcher{content: content})

	require.NoError(t, updater.updateFeeds(context.Background()))

	feeds, err := store.GetAllFeeds()
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, 0, feeds[0].ConsecutiveFailures)
	assert.Equal(t, "", feeds[0].LastError)
	assert.False(t, feeds[0].LastSuccessAt.IsZero(), "last_success_at should be set")

	// The feed title should have been updated from the fetched content.
	assert.Equal(t, "Test Feed", feeds[0].Title)
}

func TestUpdateFeeds_RecordsSuccessOnNotModified(t *testing.T) {
	store, cleanup := newUpdaterTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	_, err = store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	// stubFetcher returns nil content and nil error, mirroring a 304 Not Modified.
	updater := NewUpdaterWithFetcher(store, 30*time.Minute, 100, &stubFetcher{content: nil})

	require.NoError(t, updater.updateFeeds(context.Background()))

	feeds, err := store.GetAllFeeds()
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, 0, feeds[0].ConsecutiveFailures)
	assert.False(t, feeds[0].LastSuccessAt.IsZero(), "last_success_at should be set even on 304")
}

func TestUpdateFeeds_SkipsFeedUnderBackoff(t *testing.T) {
	store, cleanup := newUpdaterTestStore(t)
	t.Cleanup(cleanup)

	userID, err := store.GetOrCreateUser("sub", "iss")
	require.NoError(t, err)
	_, err = store.AddFeedForUser(userID, "https://example.com/feed.xml")
	require.NoError(t, err)

	feeds, err := store.GetAllFeeds()
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	feedID := feeds[0].ID

	// Seed enough recent failures to trigger backoff.
	for i := 0; i < 5; i++ {
		require.NoError(t, store.RecordFeedFailure(feedID, errors.New("boom"), time.Now()))
	}

	stub := &stubFetcher{content: &FeedContent{Title: "Should Not Be Called"}}
	updater := NewUpdaterWithFetcher(store, 30*time.Minute, 100, stub)

	require.NoError(t, updater.updateFeeds(context.Background()))

	assert.Equal(t, 0, stub.calls, "fetcher must not be called for a feed under backoff")

	// Failure state is unchanged by the skipped cycle.
	feeds, err = store.GetAllFeeds()
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, 5, feeds[0].ConsecutiveFailures)
}
