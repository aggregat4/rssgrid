package feed

import (
	"net/http"
	"testing"
	"time"

	"github.com/aggregat4/rssgrid/internal/db"
)

func TestCacheFetcher_ShouldSkipFetch(t *testing.T) {
	// Create a mock feed with cache info
	feed := &db.Feed{
		ID:         1,
		URL:        "http://example.com/feed",
		CacheUntil: time.Now().Add(1 * time.Hour), // Cache valid for 1 hour
	}

	// Should skip fetch when cache is still valid
	if !shouldSkipFetch(feed) {
		t.Error("Expected shouldSkipFetch to return true when cache is still valid")
	}

	// Should not skip fetch when cache has expired
	feed.CacheUntil = time.Now().Add(-1 * time.Hour) // Cache expired 1 hour ago
	if shouldSkipFetch(feed) {
		t.Error("Expected shouldSkipFetch to return false when cache has expired")
	}

	// Should not skip fetch when no cache info
	feed.CacheUntil = time.Time{} // Zero time
	if shouldSkipFetch(feed) {
		t.Error("Expected shouldSkipFetch to return false when no cache info")
	}
}

func TestCacheFetcher_ExtractCacheInfo(t *testing.T) {
	fetcher := &CacheFetcher{}

	// Test ETag extraction
	headers := http.Header{}
	headers.Set("ETag", `"abc123"`)
	headers.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
	headers.Set("Cache-Control", "max-age=3600")

	cacheInfo := fetcher.extractCacheInfo(headers)

	if cacheInfo.ETag != `"abc123"` {
		t.Errorf("Expected ETag to be \"abc123\", got %s", cacheInfo.ETag)
	}

	if cacheInfo.LastModified != "Wed, 21 Oct 2015 07:28:00 GMT" {
		t.Errorf("Expected Last-Modified to be \"Wed, 21 Oct 2015 07:28:00 GMT\", got %s", cacheInfo.LastModified)
	}

	// Check that cache_until is set to a future time (within reasonable bounds)
	expectedMin := time.Now().Add(3599 * time.Second) // 1 hour - 1 second
	expectedMax := time.Now().Add(3601 * time.Second) // 1 hour + 1 second
	if cacheInfo.CacheUntil.Before(expectedMin) || cacheInfo.CacheUntil.After(expectedMax) {
		t.Errorf("Expected CacheUntil to be around 1 hour from now, got %v", cacheInfo.CacheUntil)
	}
}

func TestCacheFetcher_ParseMaxAge(t *testing.T) {
	fetcher := &CacheFetcher{}

	tests := []struct {
		input    string
		expected int
	}{
		{"max-age=3600", 3600},
		{"public, max-age=1800", 1800},
		{"no-cache", 0},
		{"max-age=invalid", 0},
		{"", 0},
	}

	for _, test := range tests {
		result := fetcher.parseMaxAge(test.input)
		if result != test.expected {
			t.Errorf("parseMaxAge(%q) = %d, expected %d", test.input, result, test.expected)
		}
	}
}
