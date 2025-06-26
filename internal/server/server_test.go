package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"html/template"

	"context"

	baseliboidc "github.com/aggregat4/go-baselib-services/v3/oidc"
	"github.com/aggregat4/rssgrid/internal/db"
	"github.com/aggregat4/rssgrid/internal/templates"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

// testServer creates a test server with the given mock store
func testServer(t *testing.T, mockStore *mockStore) *Server {
	// Create a mock OIDC config
	mockOIDCConfig := &baseliboidc.OidcConfiguration{}

	// Load templates first
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create server with mock store
	server := &Server{
		store:      mockStore,
		sessions:   sessions.NewCookieStore([]byte("test-session-key")),
		fetcher:    nil, // Not needed for tests
		templates:  templates,
		oidcConfig: mockOIDCConfig,
	}

	return server
}

// createTestStore creates a temporary database store for integration tests
func createTestStore(t *testing.T) (*db.Store, func()) {
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := db.NewStore(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		os.Remove(tmpFile.Name())
	}

	return store, cleanup
}

// createTestServerWithStore creates a test server with a real store
func createTestServerWithStore(t *testing.T, store *db.Store) *Server {
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	return &Server{
		store:      store,
		sessions:   sessions.NewCookieStore([]byte("test-session-key")),
		fetcher:    nil,
		templates:  templates,
		oidcConfig: &baseliboidc.OidcConfiguration{},
	}
}

// mockStoreWithFeeds creates a mock store with the given feeds and posts
func mockStoreWithFeeds(feeds []db.Feed, posts map[int64][]db.Post) *mockStore {
	return &mockStore{
		feeds: feeds,
		posts: posts,
	}
}

// mockStoreEmpty creates a mock store with no feeds
func mockStoreEmpty() *mockStore {
	return &mockStore{
		feeds: []db.Feed{},
		posts: map[int64][]db.Post{},
	}
}

// testRequest creates a test request with a session containing the given user ID
func testRequest(server *Server, method, path string, userID int64) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, nil)

	// Create a session with a user ID
	session, _ := server.sessions.Get(req, "user_session")
	session.Values["user_id"] = userID

	// Create response recorder
	w := httptest.NewRecorder()

	return req, w
}

// assertContains checks that a string contains all expected substrings
func assertContains(t *testing.T, result, description string, expectedContent ...string) {
	for _, content := range expectedContent {
		if !strings.Contains(result, content) {
			t.Errorf("%s should contain '%s'", description, content)
		}
	}
}

// assertNotContains checks that a string does NOT contain any of the unexpected substrings
func assertNotContains(t *testing.T, result, description string, unexpectedContent ...string) {
	for _, content := range unexpectedContent {
		if strings.Contains(result, content) {
			t.Errorf("%s should not contain '%s'", description, content)
		}
	}
}

// assertResponseSuccess checks that the response has a 200 status and contains expected content
func assertResponseSuccess(t *testing.T, w *httptest.ResponseRecorder, expectedContent ...string) {
	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check that we got some content
	body := w.Body.String()
	if body == "" {
		t.Error("Response body is empty")
	}

	// Check for expected content
	assertContains(t, body, "Response body", expectedContent...)
}

// assertResponseNotContains checks that the response does NOT contain certain content
func assertResponseNotContains(t *testing.T, w *httptest.ResponseRecorder, unexpectedContent ...string) {
	body := w.Body.String()
	assertNotContains(t, body, "Response body", unexpectedContent...)
}

// assertRedirect checks that the response is a redirect to the expected location
func assertRedirect(t *testing.T, w *httptest.ResponseRecorder, expectedLocation string) {
	if w.Code != http.StatusSeeOther {
		t.Errorf("Expected status code %d, got %d", http.StatusSeeOther, w.Code)
	}

	location := w.Header().Get("Location")
	if location != expectedLocation {
		t.Errorf("Expected redirect to %s, got %s", expectedLocation, location)
	}
}

// Mock store for testing
type mockStore struct {
	feeds []db.Feed
	posts map[int64][]db.Post
}

func (m *mockStore) GetUserFeeds(userID int64) ([]db.Feed, error) {
	return m.feeds, nil
}

func (m *mockStore) GetFeedPosts(feedID, userID int64, limit int) ([]db.Post, error) {
	posts, exists := m.posts[feedID]
	if !exists {
		return []db.Post{}, nil
	}
	return posts, nil
}

func (m *mockStore) GetOrCreateUser(subject, issuer string) (int64, error) {
	return 1, nil
}

func (m *mockStore) AddFeed(url string) (int64, error) {
	return 1, nil
}

func (m *mockStore) UpdateFeedTitle(feedID int64, title string) error {
	return nil
}

func (m *mockStore) AddPost(feedID int64, guid, title, link string, publishedAt time.Time, content string) error {
	return nil
}

func (m *mockStore) DeleteFeed(feedID string) error {
	return nil
}

func (m *mockStore) MarkPostAsSeen(userID int64, postID string) error {
	return nil
}

func (m *mockStore) MarkAllFeedPostsAsSeen(userID int64, feedID string) error {
	return nil
}

func (m *mockStore) AddFeedForUser(userID int64, url string) (int64, error) {
	return 1, nil
}

func (m *mockStore) GetUserPostsPerFeed(userID int64) (int, error) {
	return 10, nil
}

func (m *mockStore) SetUserPostsPerFeed(userID int64, postsPerFeed int) error {
	return nil
}

func (m *mockStore) GetPost(postID int64) (*db.Post, error) {
	// Search through all posts to find the one with matching ID
	for _, posts := range m.posts {
		for _, post := range posts {
			if post.ID == postID {
				return &post, nil
			}
		}
	}
	return nil, fmt.Errorf("post not found")
}

func (m *mockStore) MoveFeedUp(userID int64, feedID int64) error {
	return nil
}

func (m *mockStore) MoveFeedDown(userID int64, feedID int64) error {
	return nil
}

// Test basic template loading and rendering
func TestTemplateLoading(t *testing.T) {
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Check that required templates exist
	requiredTemplates := []string{"dashboard.html", "settings.html", "post.html"}
	for _, tmplName := range requiredTemplates {
		if tmpl := templates.Lookup(tmplName); tmpl == nil {
			t.Errorf("Required template '%s' not found", tmplName)
		}
	}

	// Test rendering dashboard template with test data
	data := struct {
		Feeds []struct {
			Feed  db.Feed
			Posts []db.Post
		}
	}{
		Feeds: []struct {
			Feed  db.Feed
			Posts []db.Post
		}{
			{
				Feed: db.Feed{ID: 1, Title: "Test Feed"},
				Posts: []db.Post{
					{ID: 1, Title: "Test Post", Link: "https://example.com"},
				},
			},
		},
	}

	var buf bytes.Buffer
	err = templates.ExecuteTemplate(&buf, "dashboard.html", data)
	if err != nil {
		t.Errorf("Failed to execute dashboard template: %v", err)
	}

	result := buf.String()
	if result == "" {
		t.Error("Template execution produced empty result")
	}

	assertContains(t, result, "Template result", "Test Feed", "Test Post", "RSSGrid")
}

// Test dashboard rendering with mock data
func TestDashboardRendering(t *testing.T) {
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
		{ID: 2, URL: "https://example.com/feed2", Title: "Test Feed 2"},
	}
	posts := map[int64][]db.Post{
		1: {{ID: 1, Title: "Test Post 1", Link: "https://example.com/post1"}},
		2: {{ID: 2, Title: "Test Post 2", Link: "https://example.com/post2"}},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, posts))
	req, w := testRequest(server, "GET", "/", 1)

	server.handleDashboard(w, req)
	assertResponseSuccess(t, w, "Test Feed 1", "Test Post 1", "RSSGrid")
}

// Test settings page rendering
func TestSettingsRendering(t *testing.T) {
	tests := []struct {
		name        string
		feeds       []db.Feed
		expected    []string
		notExpected []string
	}{
		{
			name: "with feeds",
			feeds: []db.Feed{
				{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
				{ID: 2, URL: "https://example.com/feed2", Title: "Test Feed 2"},
			},
			expected: []string{"Add New Feed", "Test Feed 1", "Your Feeds", "RSSGrid"},
		},
		{
			name:        "empty",
			feeds:       []db.Feed{},
			expected:    []string{"Add New Feed", "Your Feeds", "No feeds added yet", "RSSGrid"},
			notExpected: []string{"Test Feed 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := testServer(t, mockStoreWithFeeds(tt.feeds, nil))
			req, w := testRequest(server, "GET", "/settings", 1)

			server.handleSettings(w, req)
			assertResponseSuccess(t, w, tt.expected...)
			if tt.notExpected != nil {
				assertResponseNotContains(t, w, tt.notExpected...)
			}
		})
	}
}

// Test user preferences functionality
func TestUserPreferences(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	userID, err := store.GetOrCreateUser("test-subject", "test-issuer")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	server := createTestServerWithStore(t, store)

	// Test default value
	postsPerFeed, err := store.GetUserPostsPerFeed(userID)
	if err != nil {
		t.Fatalf("Failed to get user posts per feed: %v", err)
	}
	if postsPerFeed != 10 {
		t.Errorf("Expected default posts per feed to be 10, got %d", postsPerFeed)
	}

	// Test updating preferences
	req := httptest.NewRequest("POST", "/settings/preferences", strings.NewReader("postsPerFeed=15"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	session, _ := server.sessions.Get(req, "user_session")
	session.Values["user_id"] = userID
	session.Save(req, w)

	server.handleUpdatePreferences(w, req)
	assertRedirect(t, w, "/settings")

	// Test invalid input
	req = httptest.NewRequest("POST", "/settings/preferences", strings.NewReader("postsPerFeed=invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	session, _ = server.sessions.Get(req, "user_session")
	session.Values["user_id"] = userID
	session.Save(req, w)

	server.handleUpdatePreferences(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// Test feed reordering functionality
func TestFeedReordering(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	userID, err := store.GetOrCreateUser("test-subject", "test-issuer")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Add test feeds
	feed1ID, err := store.AddFeedForUser(userID, "https://example.com/feed1.xml")
	if err != nil {
		t.Fatalf("Failed to add test feed 1: %v", err)
	}
	feed2ID, err := store.AddFeedForUser(userID, "https://example.com/feed2.xml")
	if err != nil {
		t.Fatalf("Failed to add test feed 2: %v", err)
	}
	feed3ID, err := store.AddFeedForUser(userID, "https://example.com/feed3.xml")
	if err != nil {
		t.Fatalf("Failed to add test feed 3: %v", err)
	}

	// Update feed titles
	store.UpdateFeedTitle(feed1ID, "Feed 1")
	store.UpdateFeedTitle(feed2ID, "Feed 2")
	store.UpdateFeedTitle(feed3ID, "Feed 3")

	// Test initial order
	feeds, err := store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get feeds: %v", err)
	}
	if len(feeds) != 3 {
		t.Fatalf("Expected 3 feeds, got %d", len(feeds))
	}

	// Test reordering operations
	tests := []struct {
		name      string
		operation func() error
		expected  []string
	}{
		{
			name:      "move feed 2 down",
			operation: func() error { return store.MoveFeedDown(userID, feed2ID) },
			expected:  []string{"Feed 1", "Feed 3", "Feed 2"},
		},
		{
			name:      "move feed 1 down",
			operation: func() error { return store.MoveFeedDown(userID, feed1ID) },
			expected:  []string{"Feed 3", "Feed 1", "Feed 2"},
		},
		{
			name:      "move feed 2 up",
			operation: func() error { return store.MoveFeedUp(userID, feed2ID) },
			expected:  []string{"Feed 3", "Feed 2", "Feed 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operation()
			if err != nil {
				t.Fatalf("Failed to %s: %v", tt.name, err)
			}

			feeds, err := store.GetUserFeeds(userID)
			if err != nil {
				t.Fatalf("Failed to get feeds: %v", err)
			}

			for i, expected := range tt.expected {
				if feeds[i].Title != expected {
					t.Errorf("Expected feed %d to be %s, got %s", i, expected, feeds[i].Title)
				}
			}
		})
	}
}

// Test feed lifecycle (add, delete, reorder)
func TestFeedLifecycle(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	userID, err := store.GetOrCreateUser("test-subject", "test-issuer")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	server := createTestServerWithStore(t, store)

	// Add feeds
	feed1ID, err := store.AddFeedForUser(userID, "https://example.com/feed1.xml")
	if err != nil {
		t.Fatalf("Failed to add feed 1: %v", err)
	}
	feed2ID, err := store.AddFeedForUser(userID, "https://example.com/feed2.xml")
	if err != nil {
		t.Fatalf("Failed to add feed 2: %v", err)
	}

	store.UpdateFeedTitle(feed1ID, "Feed 1")
	store.UpdateFeedTitle(feed2ID, "Feed 2")

	// Add posts
	store.AddPost(feed1ID, "guid-1", "Post 1", "https://example.com/post1", time.Now(), "")
	store.AddPost(feed2ID, "guid-2", "Post 2", "https://example.com/post2", time.Now(), "")

	// Verify dashboard shows feeds
	req, w := testRequest(server, "GET", "/", userID)
	server.handleDashboard(w, req)
	assertResponseSuccess(t, w, "Feed 1", "Feed 2", "Post 1", "Post 2")

	// Delete a feed
	err = store.DeleteFeed(fmt.Sprintf("%d", feed1ID))
	if err != nil {
		t.Fatalf("Failed to delete feed: %v", err)
	}

	// Verify dashboard shows only remaining feed
	req, w = testRequest(server, "GET", "/", userID)
	server.handleDashboard(w, req)
	assertResponseSuccess(t, w, "Feed 2", "Post 2")
	assertResponseNotContains(t, w, "Feed 1", "Post 1")

	// Verify settings page
	req, w = testRequest(server, "GET", "/settings", userID)
	server.handleSettings(w, req)
	assertResponseSuccess(t, w, "Feed 2")
	assertResponseNotContains(t, w, "Feed 1")
}

// Test move feed handlers
func TestMoveFeedHandlers(t *testing.T) {
	server := testServer(t, mockStoreWithFeeds([]db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
		{ID: 2, URL: "https://example.com/feed2", Title: "Test Feed 2"},
	}, nil))

	tests := []struct {
		name   string
		method string
		path   string
		feedID string
	}{
		{"move up", "POST", "/settings/feeds/2/move-up", "2"},
		{"move down", "POST", "/settings/feeds/1/move-down", "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			session, _ := server.sessions.Get(req, "user_session")
			session.Values["user_id"] = int64(1)
			session.Save(req, w)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("feedId", tt.feedID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			if tt.name == "move up" {
				server.handleMoveFeedUp(w, req)
			} else {
				server.handleMoveFeedDown(w, req)
			}

			assertRedirect(t, w, "/settings")
		})
	}
}

// Test logout functionality
func TestLogout(t *testing.T) {
	server := testServer(t, mockStoreEmpty())
	req, w := testRequest(server, "POST", "/logout", 1)

	server.handleLogout(w, req)
	assertRedirect(t, w, "/")
}

// Test post template rendering
func TestPostTemplateRendering(t *testing.T) {
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	testPost := struct {
		ID          int64
		Title       string
		Link        string
		PublishedAt time.Time
		Content     template.HTML
	}{
		ID:          1,
		Title:       "Test Post for Display",
		Link:        "https://example.com/post1",
		PublishedAt: time.Now(),
		Content:     template.HTML("<p>This is content for the post.</p>"),
	}

	data := struct {
		Post interface{}
	}{
		Post: testPost,
	}

	var buf bytes.Buffer
	err = templates.ExecuteTemplate(&buf, "post.html", data)
	if err != nil {
		t.Fatalf("Failed to execute post template: %v", err)
	}

	result := buf.String()
	expectedContent := []string{
		"Test Post for Display",
		"This is content for the post.",
		"View Original",
		"Close",
		"window.parent.postMessage",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected content '%s' not found in post template output", expected)
		}
	}
}

// Test dashboard template with dates and seen status
func TestDashboardTemplateWithDates(t *testing.T) {
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	testFeeds := []struct {
		Feed  db.Feed
		Posts []db.Post
	}{
		{
			Feed: db.Feed{ID: 1, Title: "Test Feed 1"},
			Posts: []db.Post{
				{ID: 1, Title: "Test Post 1", Link: "https://example.com/post1", PublishedAt: testTime, Seen: false},
				{ID: 2, Title: "Test Post 2", Link: "https://example.com/post2", PublishedAt: testTime.Add(-24 * time.Hour), Seen: true},
			},
		},
	}

	data := struct {
		Feeds []struct {
			Feed  db.Feed
			Posts []db.Post
		}
	}{
		Feeds: testFeeds,
	}

	var buf bytes.Buffer
	err = templates.ExecuteTemplate(&buf, "dashboard.html", data)
	if err != nil {
		t.Fatalf("Failed to execute dashboard template: %v", err)
	}

	result := buf.String()
	expectedContent := []string{
		"Test Feed 1",
		"Test Post 1",
		"Test Post 2",
		"January 15, 2024 at 2:30 PM",
		"January 14, 2024 at 2:30 PM",
		"RSSGrid",
		"seen",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected content '%s' not found in dashboard template output", expected)
		}
	}
}
