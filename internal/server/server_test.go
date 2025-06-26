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

func (m *mockStore) MoveFeedUp(userID int64, feedID int64) error {
	return nil
}

func (m *mockStore) MoveFeedDown(userID int64, feedID int64) error {
	return nil
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

func TestPostTemplateRendering(t *testing.T) {
	// Test that the post template renders correctly
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

	// Check for expected content
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

	t.Logf("Post template output preview: %s", result[:min(500, len(result))])
}

func TestLogout(t *testing.T) {
	server := testServer(t, mockStoreEmpty())
	req, w := testRequest(server, "POST", "/logout", 1)

	// Call the handler directly
	server.handleLogout(w, req)

	// Check that we get a redirect
	if w.Code != http.StatusSeeOther {
		t.Errorf("Expected status code %d, got %d", http.StatusSeeOther, w.Code)
	}

	// Check that we're redirected to the dashboard
	location := w.Header().Get("Location")
	if location != "/" {
		t.Errorf("Expected redirect to '/', got '%s'", location)
	}
}

func TestDashboardTemplateRendering(t *testing.T) {
	// Test that the dashboard template renders correctly with dates
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Test data with posts that have dates
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

	// Check for expected content including dates
	expectedContent := []string{
		"Test Feed 1",
		"Test Post 1",
		"Test Post 2",
		"January 15, 2024 at 2:30 PM",
		"January 14, 2024 at 2:30 PM",
		"RSSGrid",
		"seen", // Check that the seen class is applied
	}

	for _, expected := range expectedContent {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected content '%s' not found in dashboard template output", expected)
		}
	}

	t.Logf("Dashboard template output preview: %s", result[:min(500, len(result))])
}

func TestDashboardFeedLifecycle(t *testing.T) {
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

	// Load templates
	templates, err := templates.LoadTemplates()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create a mock OIDC config
	mockOIDCConfig := &baseliboidc.OidcConfiguration{}

	// Create server with real store
	server := &Server{
		store:      store,
		sessions:   sessions.NewCookieStore([]byte("test-session-key")),
		fetcher:    nil,
		templates:  templates,
		oidcConfig: mockOIDCConfig,
	}

	// Phase 1: Add initial feeds with content
	t.Log("Phase 1: Adding initial feeds")
	initialFeeds := []struct {
		url   string
		title string
		posts []struct {
			title string
			link  string
		}
	}{
		{
			url:   "https://tech.example.com/feed.xml",
			title: "Tech News",
			posts: []struct {
				title string
				link  string
			}{
				{"New AI Breakthrough", "https://tech.example.com/ai-news"},
				{"Latest Programming Trends", "https://tech.example.com/programming"},
				{"Cloud Computing Update", "https://tech.example.com/cloud"},
			},
		},
		{
			url:   "https://sports.example.com/feed.xml",
			title: "Sports Central",
			posts: []struct {
				title string
				link  string
			}{
				{"Championship Game Results", "https://sports.example.com/championship"},
				{"Player Transfer News", "https://sports.example.com/transfer"},
			},
		},
		{
			url:   "https://science.example.com/feed.xml",
			title: "Science Daily",
			posts: []struct {
				title string
				link  string
			}{
				{"Mars Mission Update", "https://science.example.com/mars"},
				{"Climate Research Findings", "https://science.example.com/climate"},
				{"Medical Breakthrough", "https://science.example.com/medical"},
			},
		},
		{
			url:   "https://cooking.example.com/feed.xml",
			title: "Cooking Corner",
			posts: []struct {
				title string
				link  string
			}{
				{"Easy Pasta Recipes", "https://cooking.example.com/pasta"},
				{"Quick Breakfast Ideas", "https://cooking.example.com/breakfast"},
			},
		},
	}

	feedIDs := make(map[string]int64)
	for _, feed := range initialFeeds {
		feedID, err := store.AddFeedForUser(userID, feed.url)
		if err != nil {
			t.Fatalf("Failed to add feed %s: %v", feed.title, err)
		}
		feedIDs[feed.title] = feedID

		// Update feed title
		err = store.UpdateFeedTitle(feedID, feed.title)
		if err != nil {
			t.Fatalf("Failed to update feed title for %s: %v", feed.title, err)
		}

		// Add posts for this feed
		for i, post := range feed.posts {
			err := store.AddPost(feedID, fmt.Sprintf("guid-%s-%d", feed.title, i), post.title, post.link, time.Now().Add(-time.Duration(i)*time.Hour), "")
			if err != nil {
				t.Fatalf("Failed to add post %s to feed %s: %v", post.title, feed.title, err)
			}
		}
	}

	// Verify initial dashboard shows all feeds and posts
	t.Log("Verifying initial dashboard")
	req, w := testRequest(server, "GET", "/", userID)
	server.handleDashboard(w, req)
	assertResponseSuccess(t, w, "Tech News", "Sports Central", "Science Daily", "Cooking Corner")
	assertResponseSuccess(t, w, "New AI Breakthrough", "Championship Game Results", "Mars Mission Update", "Easy Pasta Recipes")

	// Phase 2: Remove half of the feeds (Tech News and Sports Central)
	t.Log("Phase 2: Removing half of the feeds")
	feedsToRemove := []string{"Tech News", "Sports Central"}
	for _, feedTitle := range feedsToRemove {
		feedID := feedIDs[feedTitle]
		err := store.DeleteFeed(fmt.Sprintf("%d", feedID))
		if err != nil {
			t.Fatalf("Failed to delete feed %s: %v", feedTitle, err)
		}
	}

	// Verify dashboard shows only remaining feeds
	t.Log("Verifying dashboard after removal")
	req, w = testRequest(server, "GET", "/", userID)
	server.handleDashboard(w, req)
	assertResponseSuccess(t, w, "Science Daily", "Cooking Corner")
	assertResponseSuccess(t, w, "Mars Mission Update", "Easy Pasta Recipes")
	assertResponseNotContains(t, w, "Tech News", "Sports Central", "New AI Breakthrough", "Championship Game Results")

	// Phase 3: Add new feeds with different content
	t.Log("Phase 3: Adding new feeds")
	newFeeds := []struct {
		url   string
		title string
		posts []struct {
			title string
			link  string
		}
	}{
		{
			url:   "https://travel.example.com/feed.xml",
			title: "Travel Adventures",
			posts: []struct {
				title string
				link  string
			}{
				{"Best European Destinations", "https://travel.example.com/europe"},
				{"Budget Travel Tips", "https://travel.example.com/budget"},
				{"Adventure Tourism Guide", "https://travel.example.com/adventure"},
			},
		},
		{
			url:   "https://finance.example.com/feed.xml",
			title: "Financial Insights",
			posts: []struct {
				title string
				link  string
			}{
				{"Investment Strategies", "https://finance.example.com/investment"},
				{"Market Analysis", "https://finance.example.com/market"},
			},
		},
	}

	for _, feed := range newFeeds {
		feedID, err := store.AddFeedForUser(userID, feed.url)
		if err != nil {
			t.Fatalf("Failed to add new feed %s: %v", feed.title, err)
		}
		feedIDs[feed.title] = feedID

		// Update feed title
		err = store.UpdateFeedTitle(feedID, feed.title)
		if err != nil {
			t.Fatalf("Failed to update feed title for %s: %v", feed.title, err)
		}

		// Add posts for this feed
		for i, post := range feed.posts {
			err := store.AddPost(feedID, fmt.Sprintf("guid-%s-%d", feed.title, i), post.title, post.link, time.Now().Add(-time.Duration(i)*time.Hour), "")
			if err != nil {
				t.Fatalf("Failed to add post %s to feed %s: %v", post.title, feed.title, err)
			}
		}
	}

	// Verify final dashboard shows all current feeds and posts
	t.Log("Verifying final dashboard")
	req, w = testRequest(server, "GET", "/", userID)
	server.handleDashboard(w, req)

	// Should contain remaining original feeds
	assertResponseSuccess(t, w, "Science Daily", "Cooking Corner")
	assertResponseSuccess(t, w, "Mars Mission Update", "Easy Pasta Recipes")

	// Should contain new feeds
	assertResponseSuccess(t, w, "Travel Adventures", "Financial Insights")
	assertResponseSuccess(t, w, "Best European Destinations", "Investment Strategies")

	// Should NOT contain removed feeds
	assertResponseNotContains(t, w, "Tech News", "Sports Central", "New AI Breakthrough", "Championship Game Results")

	// Verify settings page shows correct feeds
	t.Log("Verifying settings page")
	req, w = testRequest(server, "GET", "/settings", userID)
	server.handleSettings(w, req)
	assertResponseSuccess(t, w, "Science Daily", "Cooking Corner", "Travel Adventures", "Financial Insights")
	assertResponseNotContains(t, w, "Tech News", "Sports Central")

	t.Log("Dashboard feed lifecycle test completed successfully")
}

func TestMoveFeedUp(t *testing.T) {
	// Create test data
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
		{ID: 2, URL: "https://example.com/feed2", Title: "Test Feed 2"},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, nil))

	// Create a POST request to move feed up
	req := httptest.NewRequest("POST", "/settings/feeds/2/move-up", nil)
	w := httptest.NewRecorder()

	// Create a session with a user ID
	session, _ := server.sessions.Get(req, "user_session")
	session.Values["user_id"] = int64(1)
	session.Save(req, w)

	// Set up chi router context with URL parameters
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("feedId", "2")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Call the handler directly
	server.handleMoveFeedUp(w, req)

	// Assert response - should redirect to settings
	if w.Code != http.StatusSeeOther {
		t.Errorf("Expected status code %d, got %d", http.StatusSeeOther, w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/settings" {
		t.Errorf("Expected redirect to /settings, got %s", location)
	}
}

func TestMoveFeedDown(t *testing.T) {
	// Create test data
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "Test Feed 1"},
		{ID: 2, URL: "https://example.com/feed2", Title: "Test Feed 2"},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, nil))

	// Create a POST request to move feed down
	req := httptest.NewRequest("POST", "/settings/feeds/1/move-down", nil)
	w := httptest.NewRecorder()

	// Create a session with a user ID
	session, _ := server.sessions.Get(req, "user_session")
	session.Values["user_id"] = int64(1)
	session.Save(req, w)

	// Set up chi router context with URL parameters
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("feedId", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Call the handler directly
	server.handleMoveFeedDown(w, req)

	// Assert response - should redirect to settings
	if w.Code != http.StatusSeeOther {
		t.Errorf("Expected status code %d, got %d", http.StatusSeeOther, w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/settings" {
		t.Errorf("Expected redirect to /settings, got %s", location)
	}
}

func TestFeedReorderingIntegration(t *testing.T) {
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
	err = store.UpdateFeedTitle(feed1ID, "Feed 1")
	if err != nil {
		t.Fatalf("Failed to update feed 1 title: %v", err)
	}

	err = store.UpdateFeedTitle(feed2ID, "Feed 2")
	if err != nil {
		t.Fatalf("Failed to update feed 2 title: %v", err)
	}

	err = store.UpdateFeedTitle(feed3ID, "Feed 3")
	if err != nil {
		t.Fatalf("Failed to update feed 3 title: %v", err)
	}

	// Get initial feed order
	initialFeeds, err := store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get initial feeds: %v", err)
	}

	t.Logf("Initial feed order:")
	for i, feed := range initialFeeds {
		t.Logf("  %d: %s (ID: %d, Position: %d)", i, feed.Title, feed.ID, feed.GridPosition)
	}

	// Verify initial order (should be Feed 1, Feed 2, Feed 3)
	if len(initialFeeds) != 3 {
		t.Fatalf("Expected 3 feeds, got %d", len(initialFeeds))
	}

	if initialFeeds[0].Title != "Feed 1" || initialFeeds[1].Title != "Feed 2" || initialFeeds[2].Title != "Feed 3" {
		t.Errorf("Initial order incorrect: %s, %s, %s", initialFeeds[0].Title, initialFeeds[1].Title, initialFeeds[2].Title)
	}

	// Move Feed 2 down (should become Feed 1, Feed 3, Feed 2)
	err = store.MoveFeedDown(userID, feed2ID)
	if err != nil {
		t.Fatalf("Failed to move Feed 2 down: %v", err)
	}

	// Get feeds after first move
	feedsAfterFirstMove, err := store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get feeds after first move: %v", err)
	}

	t.Logf("After moving Feed 2 down:")
	for i, feed := range feedsAfterFirstMove {
		t.Logf("  %d: %s (ID: %d, Position: %d)", i, feed.Title, feed.ID, feed.GridPosition)
	}

	// Verify order after first move
	if feedsAfterFirstMove[0].Title != "Feed 1" || feedsAfterFirstMove[1].Title != "Feed 3" || feedsAfterFirstMove[2].Title != "Feed 2" {
		t.Errorf("Order after first move incorrect: %s, %s, %s", feedsAfterFirstMove[0].Title, feedsAfterFirstMove[1].Title, feedsAfterFirstMove[2].Title)
	}

	// Move Feed 1 down (should become Feed 3, Feed 1, Feed 2)
	err = store.MoveFeedDown(userID, feed1ID)
	if err != nil {
		t.Fatalf("Failed to move Feed 1 down: %v", err)
	}

	// Get feeds after second move
	feedsAfterSecondMove, err := store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get feeds after second move: %v", err)
	}

	t.Logf("After moving Feed 1 down:")
	for i, feed := range feedsAfterSecondMove {
		t.Logf("  %d: %s (ID: %d, Position: %d)", i, feed.Title, feed.ID, feed.GridPosition)
	}

	// Verify order after second move
	if feedsAfterSecondMove[0].Title != "Feed 3" || feedsAfterSecondMove[1].Title != "Feed 1" || feedsAfterSecondMove[2].Title != "Feed 2" {
		t.Errorf("Order after second move incorrect: %s, %s, %s", feedsAfterSecondMove[0].Title, feedsAfterSecondMove[1].Title, feedsAfterSecondMove[2].Title)
	}

	// Move Feed 2 up (should become Feed 3, Feed 2, Feed 1)
	err = store.MoveFeedUp(userID, feed2ID)
	if err != nil {
		t.Fatalf("Failed to move Feed 2 up: %v", err)
	}

	// Get feeds after third move
	feedsAfterThirdMove, err := store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get feeds after third move: %v", err)
	}

	t.Logf("After moving Feed 2 up:")
	for i, feed := range feedsAfterThirdMove {
		t.Logf("  %d: %s (ID: %d, Position: %d)", i, feed.Title, feed.ID, feed.GridPosition)
	}

	// Verify order after third move
	if feedsAfterThirdMove[0].Title != "Feed 3" || feedsAfterThirdMove[1].Title != "Feed 2" || feedsAfterThirdMove[2].Title != "Feed 1" {
		t.Errorf("Order after third move incorrect: %s, %s, %s", feedsAfterThirdMove[0].Title, feedsAfterThirdMove[1].Title, feedsAfterThirdMove[2].Title)
	}

	t.Log("Feed reordering integration test completed successfully")
}

func TestDashboardHTMLFeedOrdering(t *testing.T) {
	// Create test feeds with specific grid positions
	feeds := []db.Feed{
		{ID: 1, URL: "https://example.com/feed1", Title: "First Feed", GridPosition: 1},
		{ID: 2, URL: "https://example.com/feed2", Title: "Second Feed", GridPosition: 2},
		{ID: 3, URL: "https://example.com/feed3", Title: "Third Feed", GridPosition: 3},
	}
	posts := map[int64][]db.Post{
		1: {{ID: 1, Title: "Post from First Feed", Link: "https://example.com/post1"}},
		2: {{ID: 2, Title: "Post from Second Feed", Link: "https://example.com/post2"}},
		3: {{ID: 3, Title: "Post from Third Feed", Link: "https://example.com/post3"}},
	}

	// Use a real store to test reordering logic
	tmpFile, err := os.CreateTemp("", "test-htmlorder-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := db.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	userID, err := store.GetOrCreateUser("htmltest", "issuer")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Add feeds for the user in the initial order
	for _, f := range feeds {
		_, err := store.AddFeedForUser(userID, f.URL)
		if err != nil {
			t.Fatalf("Failed to add feed: %v", err)
		}
		// Set the title and grid position
		err = store.UpdateFeedTitle(f.ID, f.Title)
		if err != nil {
			t.Fatalf("Failed to set feed title: %v", err)
		}
	}

	// Add posts for each feed
	for feedID, postsForFeed := range posts {
		for _, p := range postsForFeed {
			err := store.AddPost(feedID, fmt.Sprintf("guid-%d", p.ID), p.Title, p.Link, time.Now(), "")
			if err != nil {
				t.Fatalf("Failed to add post: %v", err)
			}
		}
	}

	// Create a real server with the real store
	srv := testServer(t, nil)
	srv.store = store

	req, w := testRequest(srv, "GET", "/", userID)
	srv.handleDashboard(w, req)
	body := w.Body.String()

	// Check initial order: First Feed, Second Feed, Third Feed
	firstFeedPos := strings.Index(body, "First Feed")
	secondFeedPos := strings.Index(body, "Second Feed")
	thirdFeedPos := strings.Index(body, "Third Feed")
	if !(firstFeedPos < secondFeedPos && secondFeedPos < thirdFeedPos) {
		t.Errorf("Initial order incorrect: First=%d, Second=%d, Third=%d", firstFeedPos, secondFeedPos, thirdFeedPos)
	}

	// Now move 'Third Feed' up (should swap with 'Second Feed')
	feedsFromDB, err := store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get user feeds: %v", err)
	}
	var thirdFeedID int64
	for _, f := range feedsFromDB {
		if f.Title == "Third Feed" {
			thirdFeedID = f.ID
		}
	}
	err = store.MoveFeedUp(userID, thirdFeedID)
	if err != nil {
		t.Fatalf("Failed to move Third Feed up: %v", err)
	}

	// Render dashboard again
	req, w = testRequest(srv, "GET", "/", userID)
	srv.handleDashboard(w, req)
	body = w.Body.String()

	// Check new order: First Feed, Third Feed, Second Feed
	firstFeedPos = strings.Index(body, "First Feed")
	thirdFeedPos = strings.Index(body, "Third Feed")
	secondFeedPos = strings.Index(body, "Second Feed")
	if !(firstFeedPos < thirdFeedPos && thirdFeedPos < secondFeedPos) {
		t.Errorf("After reordering: expected First < Third < Second, got First=%d, Third=%d, Second=%d", firstFeedPos, thirdFeedPos, secondFeedPos)
	}

	t.Logf("Feed ordering in HTML changes as expected after reordering.")
}

func TestDashboardHTMLFeedOrderingReversed(t *testing.T) {
	// Test with reversed order to ensure ordering is respected
	feeds := []db.Feed{
		{ID: 3, URL: "https://example.com/feed3", Title: "Third Feed", GridPosition: 1},
		{ID: 2, URL: "https://example.com/feed2", Title: "Second Feed", GridPosition: 2},
		{ID: 1, URL: "https://example.com/feed1", Title: "First Feed", GridPosition: 3},
	}

	posts := map[int64][]db.Post{
		1: {{ID: 1, Title: "Post from First Feed", Link: "https://example.com/post1"}},
		2: {{ID: 2, Title: "Post from Second Feed", Link: "https://example.com/post2"}},
		3: {{ID: 3, Title: "Post from Third Feed", Link: "https://example.com/post3"}},
	}

	server := testServer(t, mockStoreWithFeeds(feeds, posts))
	req, w := testRequest(server, "GET", "/", 1)

	// Call the handler directly
	server.handleDashboard(w, req)

	body := w.Body.String()

	// Check that feeds appear in the order specified by GridPosition
	thirdFeedPos := strings.Index(body, "Third Feed")
	secondFeedPos := strings.Index(body, "Second Feed")
	firstFeedPos := strings.Index(body, "First Feed")

	if thirdFeedPos == -1 || secondFeedPos == -1 || firstFeedPos == -1 {
		t.Fatal("Could not find all feed titles in HTML")
	}

	// Verify the order: Third (GridPosition 1) should come first, then Second (2), then First (3)
	if thirdFeedPos >= secondFeedPos {
		t.Errorf("Third Feed (GridPosition 1) should appear before Second Feed (GridPosition 2) in HTML. Positions: Third=%d, Second=%d",
			thirdFeedPos, secondFeedPos)
	}

	if secondFeedPos >= firstFeedPos {
		t.Errorf("Second Feed (GridPosition 2) should appear before First Feed (GridPosition 3) in HTML. Positions: Second=%d, First=%d",
			secondFeedPos, firstFeedPos)
	}

	t.Logf("Reversed feed ordering test passed. Feed positions in HTML: Third=%d, Second=%d, First=%d",
		thirdFeedPos, secondFeedPos, firstFeedPos)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
