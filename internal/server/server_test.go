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

func TestSettingsWithUserPreferences(t *testing.T) {
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
	assertResponseSuccess(t, w, "Display Settings", "Posts per feed", "Add New Feed", "Test Feed 1", "Your Feeds", "RSSGrid")
}

func TestUpdatePreferences(t *testing.T) {
	// Create test data
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, nil))

	// Create a POST request with valid form data
	req := httptest.NewRequest("POST", "/settings/preferences", strings.NewReader("postsPerFeed=15"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()

	// Create a session with a user ID
	session, _ := server.sessions.Get(req, "user_session")
	session.Values["user_id"] = int64(1)
	session.Save(req, w)

	// Call the handler directly
	server.handleUpdatePreferences(w, req)

	// Assert response - should redirect to settings
	if w.Code != http.StatusSeeOther {
		t.Errorf("Expected status code %d, got %d", http.StatusSeeOther, w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/settings" {
		t.Errorf("Expected redirect to /settings, got %s", location)
	}
}

func TestUpdatePreferencesInvalidInput(t *testing.T) {
	// Create test data
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, nil))

	// Create a POST request with invalid form data
	req := httptest.NewRequest("POST", "/settings/preferences", strings.NewReader("postsPerFeed=invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()

	// Create a session with a user ID
	session, _ := server.sessions.Get(req, "user_session")
	session.Values["user_id"] = int64(1)
	session.Save(req, w)

	// Call the handler directly
	server.handleUpdatePreferences(w, req)

	// Assert response - should return bad request
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestDashboardWithUserPreferences(t *testing.T) {
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

func TestUserPreferencesIntegration(t *testing.T) {
	// Create a temporary database
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create a real store
	store, err := db.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create a test user
	userID, err := store.GetOrCreateUser("test-subject", "test-issuer")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Add a test feed
	feedID, err := store.AddFeedForUser(userID, "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Failed to add test feed: %v", err)
	}

	// Add some test posts
	testPosts := []db.Post{
		{ID: 1, Title: "Test Post 1", Link: "https://example.com/post1"},
		{ID: 2, Title: "Test Post 2", Link: "https://example.com/post2"},
		{ID: 3, Title: "Test Post 3", Link: "https://example.com/post3"},
		{ID: 4, Title: "Test Post 4", Link: "https://example.com/post4"},
		{ID: 5, Title: "Test Post 5", Link: "https://example.com/post5"},
	}

	for _, post := range testPosts {
		err := store.AddPost(feedID, fmt.Sprintf("guid-%d", post.ID), post.Title, post.Link, time.Now(), "")
		if err != nil {
			t.Fatalf("Failed to add test post: %v", err)
		}
	}

	// Test that the user preferences are working correctly
	// First, check default value
	postsPerFeed, err := store.GetUserPostsPerFeed(userID)
	if err != nil {
		t.Fatalf("Failed to get user posts per feed: %v", err)
	}
	if postsPerFeed != 10 {
		t.Errorf("Expected default posts per feed to be 10, got %d", postsPerFeed)
	}

	// Set a custom value
	err = store.SetUserPostsPerFeed(userID, 15)
	if err != nil {
		t.Fatalf("Failed to set user posts per feed: %v", err)
	}

	// Check that the value was set correctly
	postsPerFeed, err = store.GetUserPostsPerFeed(userID)
	if err != nil {
		t.Fatalf("Failed to get user posts per feed after setting: %v", err)
	}
	if postsPerFeed != 15 {
		t.Errorf("Expected posts per feed to be 15, got %d", postsPerFeed)
	}

	// Test that the dashboard respects the user preference
	// Create a mock OIDC config
	mockOIDCConfig := &baseliboidc.OidcConfiguration{}

	// Load templates
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create server with real store
	server := &Server{
		store:      store,
		sessions:   sessions.NewCookieStore([]byte("test-session-key")),
		fetcher:    nil,
		templates:  templates,
		oidcConfig: mockOIDCConfig,
	}

	req, w := testRequest(server, "GET", "/", userID)

	// Call the handler directly
	server.handleDashboard(w, req)

	// Assert response
	assertResponseSuccess(t, w, "Test Post 1", "Test Post 2", "Test Post 3", "Test Post 4", "Test Post 5")
}

func TestPostTemplateHTMLRendering(t *testing.T) {
	// Test that HTML content is rendered correctly without escaping
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Test data with HTML content
	testPost := struct {
		ID          int64
		Title       string
		Link        string
		PublishedAt time.Time
		Content     template.HTML
	}{
		ID:          1,
		Title:       "Test Post with HTML",
		Link:        "https://example.com/post1",
		PublishedAt: time.Now(),
		Content:     template.HTML("<p>This is a <strong>bold</strong> paragraph with a <a href=\"https://example.com\">link</a>.</p><ul><li>Item 1</li><li>Item 2</li></ul>"),
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

	// Check that HTML tags are NOT escaped
	expectedHTML := []string{
		"<p>This is a <strong>bold</strong> paragraph",
		"<a href=\"https://example.com\">link</a>",
		"<ul><li>Item 1</li><li>Item 2</li></ul>",
	}

	for _, expected := range expectedHTML {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected HTML content '%s' not found in template output", expected)
		}
	}

	// Check that HTML is NOT double-escaped (should not contain &lt; or &gt;)
	escapedHTML := []string{
		"&lt;p&gt;",
		"&lt;strong&gt;",
		"&lt;a href=",
		"&lt;ul&gt;",
		"&lt;li&gt;",
	}

	for _, escaped := range escapedHTML {
		if strings.Contains(result, escaped) {
			t.Errorf("Found escaped HTML '%s' in template output, HTML should not be escaped", escaped)
		}
	}

	t.Logf("Template output preview: %s", result[:min(500, len(result))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
