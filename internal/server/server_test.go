package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	baseliboidc "github.com/aggregat4/go-baselib-services/v3/oidc"
	"github.com/aggregat4/rssgrid/internal/db"
	"github.com/aggregat4/rssgrid/internal/templates"
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

	// Check that the template content is present
	if len(body) < 100 {
		t.Errorf("Response body too short: %d characters", len(body))
	}

	// Check for expected content
	assertContains(t, body, "Response body", expectedContent...)

	t.Logf("Response body length: %d characters", len(body))
	t.Logf("Response body preview: %s", body[:min(200, len(body))])
}

// assertResponseNotContains checks that the response does NOT contain certain content
func assertResponseNotContains(t *testing.T, w *httptest.ResponseRecorder, unexpectedContent ...string) {
	body := w.Body.String()
	assertNotContains(t, body, "Response body", unexpectedContent...)
}

func TestDashboardRendering(t *testing.T) {
	// Create test data
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
		{ID: 2, URL: "https://example.com/feed2", Title: "Test Feed 2"},
	}
	posts := map[int64][]db.Post{
		1: {
			{ID: 1, Title: "Test Post 1", Link: "https://example.com/post1"},
			{ID: 2, Title: "Test Post 2", Link: "https://example.com/post2"},
		},
		2: {
			{ID: 3, Title: "Test Post 3", Link: "https://example.com/post3"},
		},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, posts))
	req, w := testRequest(server, "GET", "/", 1)

	// Call the handler directly
	server.handleDashboard(w, req)

	// Assert response
	assertResponseSuccess(t, w, "Test Feed 1", "Test Post 1", "RSSGrid")
}

func TestTemplateLoading(t *testing.T) {
	// Test that templates load correctly
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Check that required templates exist
	requiredTemplates := []string{"dashboard.html", "settings.html"}
	for _, tmplName := range requiredTemplates {
		if tmpl := templates.Lookup(tmplName); tmpl == nil {
			t.Errorf("Required template '%s' not found", tmplName)
		} else {
			t.Logf("Template '%s' loaded successfully", tmplName)
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

	// Check for expected content
	assertContains(t, result, "Template result", "Test Feed", "Test Post", "RSSGrid")

	t.Logf("Template execution result length: %d", len(result))
	t.Logf("Template execution preview: %s", result[:min(200, len(result))])
}

func TestSettingsRendering(t *testing.T) {
	// Create test data
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
		{ID: 2, URL: "https://example.com/feed2", Title: "Test Feed 2"},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, nil))
	req, w := testRequest(server, "GET", "/settings", 1)

	// Call the handler directly
	server.handleSettings(w, req)

	// Assert response
	assertResponseSuccess(t, w, "Add New Feed", "Test Feed 1", "Your Feeds", "RSSGrid")
}

func TestSettingsRenderingEmpty(t *testing.T) {
	server := testServer(t, mockStoreEmpty())
	req, w := testRequest(server, "GET", "/settings", 1)

	// Call the handler directly
	server.handleSettings(w, req)

	// Assert response
	assertResponseSuccess(t, w, "Add New Feed", "Your Feeds", "No feeds added yet", "RSSGrid")
	assertResponseNotContains(t, w, "Test Feed 1")
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
