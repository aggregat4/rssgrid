package feed

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

type Fetcher struct {
	client    *http.Client
	parser    *gofeed.Parser
	sanitizer *bluemonday.Policy
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		parser:    gofeed.NewParser(),
		sanitizer: bluemonday.UGCPolicy(),
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

func (f *Fetcher) FetchFeed(ctx context.Context, url string) (*FeedContent, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add common headers
	req.Header.Set("User-Agent", "RSSGrid/1.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned non-200 status code: %d", resp.StatusCode)
	}

	feed, err := f.parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error parsing feed: %w", err)
	}

	content := &FeedContent{
		Title: feed.Title,
		Items: make([]FeedItem, 0, len(feed.Items)),
	}

	if feed.UpdatedParsed != nil {
		content.LastUpdated = *feed.UpdatedParsed
	} else if feed.PublishedParsed != nil {
		content.LastUpdated = *feed.PublishedParsed
	}

	for _, item := range feed.Items {
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

		// Sanitize content
		sanitizedContent := f.sanitizer.Sanitize(postContent)

		content.Items = append(content.Items, FeedItem{
			GUID:        guid,
			Title:       item.Title,
			Link:        item.Link,
			PublishedAt: publishedAt,
			Content:     sanitizedContent,
		})
	}

	return content, nil
}
