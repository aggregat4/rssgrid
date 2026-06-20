package db

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAuthTestStore creates a fresh store backed by a temp SQLite file and
// returns it together with a cleanup function.
func newAuthTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "auth-test-*.db")
	require.NoError(t, err)
	_ = tmpFile.Close()
	store, err := NewStore(tmpFile.Name())
	require.NoError(t, err)
	return store, func() {
		_ = store.db.Close()
		_ = os.Remove(tmpFile.Name())
	}
}

// setupTwoUsersWithFeeds creates two users, each subscribed to their own feed
// with one post, and returns the ids. A third shared feed is subscribed to by
// both users to exercise the "feed kept alive by another subscriber" case.
type authFixture struct {
	store   *Store
	user1   int64
	user2   int64
	feed1   int64 // owned by user1 only
	feed2   int64 // owned by user2 only
	shared  int64 // owned by both users
	post1   int64 // in feed1
	post2   int64 // in feed2
	sharedP int64 // in shared
}

func setupTwoUsersWithFeeds(t *testing.T) *authFixture {
	t.Helper()
	store, cleanup := newAuthTestStore(t)
	t.Cleanup(cleanup)

	user1, err := store.GetOrCreateUser("subject1", "issuer1")
	require.NoError(t, err)
	user2, err := store.GetOrCreateUser("subject2", "issuer2")
	require.NoError(t, err)

	feed1, err := store.AddFeedForUser(user1, "https://example.com/feed1.xml")
	require.NoError(t, err)
	feed2, err := store.AddFeedForUser(user2, "https://example.com/feed2.xml")
	require.NoError(t, err)
	shared, err := store.AddFeedForUser(user1, "https://example.com/shared.xml")
	require.NoError(t, err)
	_, err = store.AddFeedForUser(user2, "https://example.com/shared.xml")
	require.NoError(t, err)

	require.NoError(t, store.AddPost(feed1, "g1", "Post 1", "https://example.com/p1", time.Now(), "c1"))
	require.NoError(t, store.AddPost(feed2, "g2", "Post 2", "https://example.com/p2", time.Now(), "c2"))
	require.NoError(t, store.AddPost(shared, "gs", "Shared Post", "https://example.com/ps", time.Now(), "cs"))

	var post1, post2, sharedP int64
	require.NoError(t, store.db.QueryRow("SELECT id FROM posts WHERE guid = 'g1'").Scan(&post1))
	require.NoError(t, store.db.QueryRow("SELECT id FROM posts WHERE guid = 'g2'").Scan(&post2))
	require.NoError(t, store.db.QueryRow("SELECT id FROM posts WHERE guid = 'gs'").Scan(&sharedP))

	return &authFixture{
		store:   store,
		user1:   user1,
		user2:   user2,
		feed1:   feed1,
		feed2:   feed2,
		shared:  shared,
		post1:   post1,
		post2:   post2,
		sharedP: sharedP,
	}
}

func TestGetPostForUser_OwnedPost(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	post, err := f.store.GetPostForUser(f.user1, f.post1)
	require.NoError(t, err)
	assert.Equal(t, "Post 1", post.Title)
}

func TestGetPostForUser_NotOwnedReturnsErrNoRows(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	// user1 tries to read a post that belongs to user2's feed
	_, err := f.store.GetPostForUser(f.user1, f.post2)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestGetPostForUser_NonexistentPostReturnsErrNoRows(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	_, err := f.store.GetPostForUser(f.user1, 999999)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestMarkPostAsSeenForUser_Owned(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	require.NoError(t, f.store.MarkPostAsSeenForUser(f.user1, f.post1))

	var seen int
	require.NoError(t, f.store.db.QueryRow(
		"SELECT seen FROM user_post_states WHERE user_id = ? AND post_id = ?",
		f.user1, f.post1).Scan(&seen))
	assert.Equal(t, 1, seen)
}

func TestMarkPostAsSeenForUser_NotOwnedReturnsErrNoRows(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	// user1 cannot mark a post from user2's feed as seen
	err := f.store.MarkPostAsSeenForUser(f.user1, f.post2)
	assert.ErrorIs(t, err, sql.ErrNoRows)

	// No state row should have been created for user1 on post2
	var count int
	require.NoError(t, f.store.db.QueryRow(
		"SELECT COUNT(*) FROM user_post_states WHERE user_id = ? AND post_id = ?",
		f.user1, f.post2).Scan(&count))
	assert.Equal(t, 0, count, "no seen-state should be recorded for a non-owned post")
}

func TestMarkAllFeedPostsAsSeenForUser_Owned(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	require.NoError(t, f.store.MarkAllFeedPostsAsSeenForUser(f.user1, f.feed1))

	var count int
	require.NoError(t, f.store.db.QueryRow(
		"SELECT COUNT(*) FROM user_post_states WHERE user_id = ? AND seen = 1",
		f.user1).Scan(&count))
	assert.GreaterOrEqual(t, count, 1)
}

func TestMarkAllFeedPostsAsSeenForUser_NotOwnedReturnsErrNoRows(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	// user1 cannot mark posts in user2's feed as seen
	err := f.store.MarkAllFeedPostsAsSeenForUser(f.user1, f.feed2)
	assert.ErrorIs(t, err, sql.ErrNoRows)

	var count int
	require.NoError(t, f.store.db.QueryRow(
		"SELECT COUNT(*) FROM user_post_states WHERE user_id = ?", f.user1).Scan(&count))
	assert.Equal(t, 0, count, "no seen-states should be recorded for a non-owned feed")
}

func TestDeleteFeedForUser_RemovesSubscriptionOnly(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	// Both users subscribe to the shared feed. user1 unsubscribes; the feed
	// must survive because user2 still subscribes.
	require.NoError(t, f.store.DeleteFeedForUser(f.user1, f.shared))

	// user1 no longer sees the shared feed
	user1Feeds, err := f.store.GetUserFeeds(f.user1)
	require.NoError(t, err)
	for _, fl := range user1Feeds {
		assert.NotEqual(t, f.shared, fl.ID, "shared feed should be gone for user1")
	}

	// user2 still sees the shared feed
	user2Feeds, err := f.store.GetUserFeeds(f.user2)
	require.NoError(t, err)
	var found bool
	for _, fl := range user2Feeds {
		if fl.ID == f.shared {
			found = true
		}
	}
	assert.True(t, found, "shared feed should still exist for user2")

	// The feed row itself must still exist
	var feedCount int
	require.NoError(t, f.store.db.QueryRow("SELECT COUNT(*) FROM feeds WHERE id = ?", f.shared).Scan(&feedCount))
	assert.Equal(t, 1, feedCount, "feed row should be retained while a subscriber remains")
}

func TestDeleteFeedForUser_GCsFeedWhenNoSubscribersRemain(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	// user2 unsubscribes from shared, then user1 unsubscribes -> feed deleted
	require.NoError(t, f.store.DeleteFeedForUser(f.user2, f.shared))
	require.NoError(t, f.store.DeleteFeedForUser(f.user1, f.shared))

	var feedCount int
	require.NoError(t, f.store.db.QueryRow("SELECT COUNT(*) FROM feeds WHERE id = ?", f.shared).Scan(&feedCount))
	assert.Equal(t, 0, feedCount, "feed row should be deleted when no subscribers remain")

	var postCount int
	require.NoError(t, f.store.db.QueryRow("SELECT COUNT(*) FROM posts WHERE feed_id = ?", f.shared).Scan(&postCount))
	assert.Equal(t, 0, postCount, "posts should cascade-delete with the feed")
}

func TestDeleteFeedForUser_NotSubscribedReturnsErrNoRows(t *testing.T) {
	f := setupTwoUsersWithFeeds(t)

	// user1 is not subscribed to feed2 (user2's feed)
	err := f.store.DeleteFeedForUser(f.user1, f.feed2)
	assert.ErrorIs(t, err, sql.ErrNoRows)

	// feed2 survives for user2
	user2Feeds, err := f.store.GetUserFeeds(f.user2)
	require.NoError(t, err)
	var found bool
	for _, fl := range user2Feeds {
		if fl.ID == f.feed2 {
			found = true
		}
	}
	assert.True(t, found, "feed2 should still exist for user2")
}
