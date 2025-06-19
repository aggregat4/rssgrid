package feed

import (
	"context"
	"log"
	"time"

	"github.com/aggregat4/rssgrid/internal/db"
)

type Updater struct {
	store   *db.Store
	fetcher *CacheFetcher
	ticker  *time.Ticker
	done    chan bool
}

func NewUpdater(store *db.Store, interval time.Duration) *Updater {
	return &Updater{
		store:   store,
		fetcher: NewCacheFetcher(store),
		ticker:  time.NewTicker(interval),
		done:    make(chan bool),
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
	// Get all unique feed URLs
	feeds, err := u.store.GetAllFeeds()
	if err != nil {
		return err
	}

	for _, feed := range feeds {
		// Fetch and parse feed with cache awareness
		result, err := u.fetcher.FetchFeedWithCache(ctx, feed.URL)
		if err != nil {
			log.Printf("Error fetching feed %s: %v", feed.URL, err)
			continue
		}

		// If no content returned, feed was cached or not modified
		if result.Content == nil {
			log.Printf("Feed %s was cached or not modified, skipping", feed.URL)
			continue
		}

		// Update feed title if it has changed
		if result.Content.Title != feed.Title {
			if err := u.store.UpdateFeedTitle(feed.ID, result.Content.Title); err != nil {
				log.Printf("Error updating feed title: %v", err)
			}
		}

		// Add new posts
		for _, item := range result.Content.Items {
			if err := u.store.AddPost(feed.ID, item.GUID, item.Title, item.Link, item.PublishedAt, item.Content); err != nil {
				log.Printf("Error adding post: %v", err)
			}
		}

		// Update last fetched timestamp
		if err := u.store.UpdateFeedLastFetched(feed.ID, time.Now()); err != nil {
			log.Printf("Error updating feed last fetched: %v", err)
		}

		// Update cache information if we should cache
		if result.ShouldCache && result.CacheInfo != nil {
			if err := u.fetcher.UpdateFeedCache(feed.ID, result.CacheInfo); err != nil {
				log.Printf("Error updating feed cache info: %v", err)
			}
		}
	}

	return nil
}
