package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/aggregat4/rssgrid/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serverAuthFixture stands up a real store-backed server with two users, each
// owning a feed with a post.
type serverAuthFixture struct {
	server *Server
	store  *db.Store
	user1  int64
	user2  int64
	feed1  int64 // user1's feed
	feed2  int64 // user2's feed
	post1  int64 // in feed1
	post2  int64 // in feed2
}

func newServerAuthFixture(t *testing.T) *serverAuthFixture {
	t.Helper()
	store, cleanup := createTestStore(t)
	t.Cleanup(cleanup)

	user1, err := store.GetOrCreateUser("sub1", "iss1")
	require.NoError(t, err)
	user2, err := store.GetOrCreateUser("sub2", "iss2")
	require.NoError(t, err)

	feed1, err := store.AddFeedForUser(user1, "https://example.com/feed1.xml")
	require.NoError(t, err)
	feed2, err := store.AddFeedForUser(user2, "https://example.com/feed2.xml")
	require.NoError(t, err)

	require.NoError(t, store.AddPost(feed1, "g1", "Post 1", "https://example.com/p1", time.Now(), "content 1"))
	require.NoError(t, store.AddPost(feed2, "g2", "Post 2", "https://example.com/p2", time.Now(), "content 2"))

	// Resolve post IDs via the public store API.
	p1, err := store.GetFeedPosts(feed1, user1, 10)
	require.NoError(t, err)
	require.Len(t, p1, 1)
	p2, err := store.GetFeedPosts(feed2, user2, 10)
	require.NoError(t, err)
	require.Len(t, p2, 1)

	return &serverAuthFixture{
		server: createTestServerWithStore(t, store),
		store:  store,
		user1:  user1,
		user2:  user2,
		feed1:  feed1,
		feed2:  feed2,
		post1:  p1[0].ID,
		post2:  p2[0].ID,
	}
}

// requestAs builds a request authenticated as the given user, with the chi
// URL params populated from the given map.
func requestAs(server *Server, method, path string, userID int64, params map[string]string) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	session, _ := server.sessions.Get(req, "user_session")
	session.Values["user_id"] = userID
	_ = session.Save(req, w)

	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	return req, w
}

func TestHandleGetPost_CrossUserDenied(t *testing.T) {
	f := newServerAuthFixture(t)

	// user1 reads own post: 200
	req, w := requestAs(f.server, "GET", "/posts/"+strconv.FormatInt(f.post1, 10), f.user1,
		map[string]string{"postId": strconv.FormatInt(f.post1, 10)})
	f.server.handleGetPost(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// user1 reads user2's post: 404, and the post still exists for user2
	req, w = requestAs(f.server, "GET", "/posts/"+strconv.FormatInt(f.post2, 10), f.user1,
		map[string]string{"postId": strconv.FormatInt(f.post2, 10)})
	f.server.handleGetPost(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	posts, err := f.store.GetFeedPosts(f.feed2, f.user2, 10)
	require.NoError(t, err)
	assert.Len(t, posts, 1, "user2's post must still be present")
}

func TestHandleMarkPostSeen_CrossUserDenied(t *testing.T) {
	f := newServerAuthFixture(t)

	// user1 marks own post: 200 and the post becomes seen
	req, w := requestAs(f.server, "POST", "/posts/"+strconv.FormatInt(f.post1, 10)+"/seen", f.user1,
		map[string]string{"postId": strconv.FormatInt(f.post1, 10)})
	f.server.handleMarkPostSeen(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	seen, err := f.store.GetFeedPosts(f.feed1, f.user1, 10)
	require.NoError(t, err)
	require.Len(t, seen, 1)
	assert.True(t, seen[0].Seen, "user1's own post should be marked seen")

	// user1 marks user2's post: 404. Because the store returns ErrNoRows only
	// when no row was inserted, this also proves no seen-state was recorded.
	req, w = requestAs(f.server, "POST", "/posts/"+strconv.FormatInt(f.post2, 10)+"/seen", f.user1,
		map[string]string{"postId": strconv.FormatInt(f.post2, 10)})
	f.server.handleMarkPostSeen(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleMarkAllSeen_CrossUserDenied(t *testing.T) {
	f := newServerAuthFixture(t)

	// user1 marks own feed: redirect and its post becomes seen
	req, w := requestAs(f.server, "POST", "/feeds/"+strconv.FormatInt(f.feed1, 10)+"/seen", f.user1,
		map[string]string{"feedId": strconv.FormatInt(f.feed1, 10)})
	f.server.handleMarkAllSeen(w, req)
	assert.Equal(t, http.StatusSeeOther, w.Code)

	seen, err := f.store.GetFeedPosts(f.feed1, f.user1, 10)
	require.NoError(t, err)
	require.Len(t, seen, 1)
	assert.True(t, seen[0].Seen, "user1's own feed post should be marked seen")

	// user1 marks user2's feed: 404. The store returns ErrNoRows only when the
	// user is not subscribed, so no seen-states are recorded for user1.
	req, w = requestAs(f.server, "POST", "/feeds/"+strconv.FormatInt(f.feed2, 10)+"/seen", f.user1,
		map[string]string{"feedId": strconv.FormatInt(f.feed2, 10)})
	f.server.handleMarkAllSeen(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// user2's post remains unseen from user2's perspective (unaffected).
	user2Posts, err := f.store.GetFeedPosts(f.feed2, f.user2, 10)
	require.NoError(t, err)
	require.Len(t, user2Posts, 1)
	assert.False(t, user2Posts[0].Seen, "user2's post should be unaffected")
}

func TestHandleDeleteFeed_CrossUserDenied(t *testing.T) {
	f := newServerAuthFixture(t)

	// user1 tries to delete user2's feed: 404
	req, w := requestAs(f.server, "POST", "/settings/feeds/"+strconv.FormatInt(f.feed2, 10)+"/delete", f.user1,
		map[string]string{"feedId": strconv.FormatInt(f.feed2, 10)})
	f.server.handleDeleteFeed(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// user2's feed must still exist and be visible to user2
	feeds, err := f.store.GetUserFeeds(f.user2)
	require.NoError(t, err)
	var found bool
	for _, fl := range feeds {
		if fl.ID == f.feed2 {
			found = true
		}
	}
	assert.True(t, found, "user2's feed must survive the cross-user delete attempt")

	// user1 deleting their own feed: redirect, and it's gone for user1
	req, w = requestAs(f.server, "POST", "/settings/feeds/"+strconv.FormatInt(f.feed1, 10)+"/delete", f.user1,
		map[string]string{"feedId": strconv.FormatInt(f.feed1, 10)})
	f.server.handleDeleteFeed(w, req)
	assert.Equal(t, http.StatusSeeOther, w.Code)

	feeds, err = f.store.GetUserFeeds(f.user1)
	require.NoError(t, err)
	assert.Empty(t, feeds, "user1 should have no feeds left after deleting their own")
}
