package feed

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aggregat4/rssgrid/internal/db"
	"github.com/mmcdole/gofeed"
)

type CacheFetcher struct {
	client *http.Client
	parser *gofeed.Parser
	store  *db.Store
}

func NewCacheFetcher(store *db.Store) *CacheFetcher {
	return &CacheFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		parser: gofeed.NewParser(),
		store:  store,
	}
}

type FeedContent struct {
	Title       string
	Items       []FeedItem
	LastUpdated time.Time
}

type FeedItem struct {
	GUID        string
	Title       string
	Link        string
	PublishedAt time.Time
	Content     string
}

type FetchResult struct {
	Content     *FeedContent
	ShouldCache bool
	CacheInfo   *CacheInfo
	Error       error
}

func (f *CacheFetcher) FetchFeedWithCache(ctx context.Context, url string) (*FetchResult, error) {
	// Check if we have cached information for this feed
	feed, err := f.store.GetFeedByURL(url)
	if err != nil {
		return nil, fmt.Errorf("error checking feed cache: %w", err)
	}

	// If we have cache info, check if we should skip fetching
	if feed != nil {
		if shouldSkipFetch(feed) {
			return &FetchResult{
				Content:     nil,
				ShouldCache: false,
				Error:       nil,
			}, nil
		}
	}

	// Create request with cache headers if available
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add common headers
	req.Header.Set("User-Agent", "RSSGrid/1.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/json")

	// Add cache headers if we have them
	if feed != nil {
		if feed.ETag != "" {
			req.Header.Set("If-None-Match", feed.ETag)
		}
		if feed.LastModified != "" {
			req.Header.Set("If-Modified-Since", feed.LastModified)
		}
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching feed: %w", err)
	}
	defer resp.Body.Close()

	// Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		return &FetchResult{
			Content:     nil,
			ShouldCache: false,
			Error:       nil,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned non-200 status code: %d", resp.StatusCode)
	}

	feedContent, err := f.parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error parsing feed: %w", err)
	}

	content := &FeedContent{
		Title: feedContent.Title,
		Items: make([]FeedItem, 0, len(feedContent.Items)),
	}

	if feedContent.UpdatedParsed != nil {
		content.LastUpdated = *feedContent.UpdatedParsed
	} else if feedContent.PublishedParsed != nil {
		content.LastUpdated = *feedContent.PublishedParsed
	}

	for _, item := range feedContent.Items {
		// Determine GUID
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		// Determine published time
		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}

		// Get content
		postContent := item.Content
		if postContent == "" {
			postContent = item.Description
		}

		content.Items = append(content.Items, FeedItem{
			GUID:        guid,
			Title:       item.Title,
			Link:        item.Link,
			PublishedAt: publishedAt,
			Content:     postContent,
		})
	}

	// Extract cache information from response headers
	cacheInfo := f.extractCacheInfo(resp.Header)

	return &FetchResult{
		Content:     content,
		ShouldCache: true,
		CacheInfo:   cacheInfo,
		Error:       nil,
	}, nil
}

type CacheInfo struct {
	ETag         string
	LastModified string
	CacheUntil   time.Time
}

func (f *CacheFetcher) extractCacheInfo(headers http.Header) *CacheInfo {
	info := &CacheInfo{
		CacheUntil: time.Now().Add(1 * time.Hour), // Default to 1 hour
	}

	// Extract ETag
	if etag := headers.Get("ETag"); etag != "" {
		info.ETag = etag
	}

	// Extract Last-Modified
	if lastModified := headers.Get("Last-Modified"); lastModified != "" {
		info.LastModified = lastModified
	}

	// Parse Cache-Control header
	if cacheControl := headers.Get("Cache-Control"); cacheControl != "" {
		if maxAge := f.parseMaxAge(cacheControl); maxAge > 0 {
			info.CacheUntil = time.Now().Add(time.Duration(maxAge) * time.Second)
		}
	}

	// Parse Expires header (takes precedence over Cache-Control)
	if expires := headers.Get("Expires"); expires != "" {
		if parsedTime, err := time.Parse(time.RFC1123, expires); err == nil {
			info.CacheUntil = parsedTime
		}
	}

	return info
}

func (f *CacheFetcher) parseMaxAge(cacheControl string) int {
	parts := strings.Split(cacheControl, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "max-age=") {
			if maxAgeStr := strings.TrimPrefix(part, "max-age="); maxAgeStr != "" {
				if maxAge, err := strconv.Atoi(maxAgeStr); err == nil {
					return maxAge
				}
			}
		}
	}
	return 0
}

func shouldSkipFetch(feed *db.Feed) bool {
	// Check if cache hasn't expired yet
	if !feed.CacheUntil.IsZero() && time.Now().Before(feed.CacheUntil) {
		return true
	}
	return false
}

// UpdateFeedCache updates the cache information for a feed in the database
func (f *CacheFetcher) UpdateFeedCache(feedID int64, cacheInfo *CacheInfo) error {
	return f.store.UpdateFeedCacheInfo(feedID, cacheInfo.ETag, cacheInfo.LastModified, cacheInfo.CacheUntil)
}
