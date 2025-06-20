package feed

import (
	"context"
	"log"
	"time"

	"github.com/aggregat4/rssgrid/internal/db"
)

type Updater struct {
	store   *db.Store
	fetcher *Fetcher
	ticker  *time.Ticker
	done    chan bool
}

func NewUpdater(store *db.Store, interval time.Duration) *Updater {
	return &Updater{
		store:   store,
		fetcher: NewFetcher(store),
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
	log.Printf("Starting feed update cycle")

	// Get all unique feed URLs
	feeds, err := u.store.GetAllFeeds()
	if err != nil {
		return err
	}

	log.Printf("Found %d feeds to update", len(feeds))

	for _, feed := range feeds {
		log.Printf("Updating feed: %s (%s)", feed.Title, feed.URL)

		// Fetch and parse feed with cache awareness
		content, err := u.fetcher.FetchFeed(ctx, feed.URL)
		if err != nil {
			log.Printf("Error fetching feed %s: %v", feed.URL, err)
			continue
		}

		// If no content returned, feed was cached or not modified
		if content == nil {
			log.Printf("Feed %s was cached or not modified, skipping", feed.URL)
			continue
		}

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

		// Update last fetched timestamp
		if err := u.store.UpdateFeedLastFetched(feed.ID, time.Now()); err != nil {
			log.Printf("Error updating feed last fetched: %v", err)
		}
	}

	log.Printf("Feed update cycle completed")
	return nil
}
