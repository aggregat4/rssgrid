package feed

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/aggregat4/rssgrid/internal/db"
)

// FeedFetcher abstracts fetching a single feed by URL, so the updater can be
// tested without hitting the network. *Fetcher satisfies it.
type FeedFetcher interface {
	FetchFeed(ctx context.Context, url string) (*FeedContent, error)
}

// backoffThreshold is the number of consecutive failures after which the
// updater starts backing off before retrying a feed.
const backoffThreshold = 5

// maxBackoff caps the exponential backoff window applied to failing feeds.
const maxBackoff = 24 * time.Hour

type Updater struct {
	store           *db.Store
	fetcher         FeedFetcher
	interval        time.Duration
	ticker          *time.Ticker
	done            chan bool
	maxPostsPerFeed int
}

func NewUpdater(store *db.Store, interval time.Duration, maxPostsPerFeed int) *Updater {
	return &Updater{
		store:           store,
		fetcher:         NewFetcher(store),
		interval:        interval,
		ticker:          time.NewTicker(interval),
		done:            make(chan bool),
		maxPostsPerFeed: maxPostsPerFeed,
	}
}

// NewUpdaterWithFetcher constructs an Updater that uses the given fetcher,
// primarily for tests. The ticker is not started by this constructor.
func NewUpdaterWithFetcher(store *db.Store, interval time.Duration, maxPostsPerFeed int, fetcher FeedFetcher) *Updater {
	return &Updater{
		store:           store,
		fetcher:         fetcher,
		interval:        interval,
		ticker:          time.NewTicker(interval),
		done:            make(chan bool),
		maxPostsPerFeed: maxPostsPerFeed,
	}
}

func (u *Updater) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-u.ticker.C:
				if err := u.updateFeeds(ctx); err != nil {
					log.Printf("Error updating feeds: %v", err)
				}
			case <-u.done:
				u.ticker.Stop()
				return
			case <-ctx.Done():
				u.ticker.Stop()
				return
			}
		}
	}()
}

func (u *Updater) Stop() {
	u.done <- true
}

func (u *Updater) updateFeeds(ctx context.Context) error {
	log.Printf("Starting feed update cycle")

	// Get all unique feed URLs
	feeds, err := u.store.GetAllFeeds()
	if err != nil {
		return err
	}

	log.Printf("Found %d feeds to update", len(feeds))

	now := time.Now()
	for _, feed := range feeds {
		if shouldBackOff(feed, now, u.interval) {
			log.Printf("Skipping feed %s (%s): backing off after %d consecutive failures",
				feed.Title, feed.URL, feed.ConsecutiveFailures)
			continue
		}

		log.Printf("Updating feed: %s (%s)", feed.Title, feed.URL)

		// Fetch and parse feed with cache awareness
		content, err := u.fetcher.FetchFeed(ctx, feed.URL)
		if err != nil {
			log.Printf("Error fetching feed %s: %v", feed.URL, err)
			if recordErr := u.store.RecordFeedFailure(feed.ID, err, time.Now()); recordErr != nil {
				log.Printf("Error recording feed failure for %s: %v", feed.URL, recordErr)
			}
			continue
		}

		// A successful fetch (whether or not it returned new content) clears
		// the failure state and records the success time.
		if recordErr := u.store.RecordFeedSuccess(feed.ID, time.Now()); recordErr != nil {
			log.Printf("Error recording feed success for %s: %v", feed.URL, recordErr)
		}

		// If no content returned, feed was cached or not modified
		if content == nil {
			log.Printf("Feed %s was cached or not modified, skipping", feed.URL)
		} else {
			u.ingestContent(feed, content)
		}

		// Prune old posts to prevent unbounded database growth
		if err := u.store.PruneFeedPosts(feed.ID, u.maxPostsPerFeed); err != nil {
			log.Printf("Error pruning posts for feed %s: %v", feed.Title, err)
		}

		// Update last fetched timestamp
		if err := u.store.UpdateFeedLastFetched(feed.ID, time.Now()); err != nil {
			log.Printf("Error updating feed last fetched: %v", err)
		}
	}

	log.Printf("Feed update cycle completed")
	return nil
}

// ingestContent updates the feed title if it changed and adds any new posts.
func (u *Updater) ingestContent(feed db.Feed, content *FeedContent) {
	// Update feed title if it has changed
	if content.Title != feed.Title {
		log.Printf("Updating feed title from '%s' to '%s'", feed.Title, content.Title)
		if err := u.store.UpdateFeedTitle(feed.ID, content.Title); err != nil {
			log.Printf("Error updating feed title: %v", err)
		}
	}

	// Add new posts
	newPostsCount := 0
	for _, item := range content.Items {
		if err := u.store.AddPost(feed.ID, item.GUID, item.Title, item.Link, item.PublishedAt, item.Content); err != nil {
			log.Printf("Error adding post: %v", err)
		} else {
			newPostsCount++
		}
	}

	if newPostsCount > 0 {
		log.Printf("Added %d new posts from feed: %s", newPostsCount, feed.Title)
	}
}

// shouldBackOff reports whether a feed should be skipped this cycle because it
// has been failing repeatedly and the exponential backoff window has not yet
// elapsed. The backoff is 2^(failures) * interval, capped at maxBackoff, and
// only applies once ConsecutiveFailures reaches backoffThreshold.
func shouldBackOff(feed db.Feed, now time.Time, interval time.Duration) bool {
	if feed.ConsecutiveFailures < backoffThreshold {
		return false
	}
	if feed.LastErrorAt.IsZero() {
		return false
	}
	backoff := time.Duration(math.Pow(2, float64(feed.ConsecutiveFailures))) * interval
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	return now.Before(feed.LastErrorAt.Add(backoff))
}
