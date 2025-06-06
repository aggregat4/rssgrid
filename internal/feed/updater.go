package feed

import (
	"context"
	"log"
	"time"

	"github.com/boris/go-rssgrid/internal/db"
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
		fetcher: NewFetcher(),
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
		// Skip if feed was recently updated
		if time.Since(feed.LastFetchedAt) < 30*time.Minute {
			continue
		}

		// Fetch and parse feed
		content, err := u.fetcher.FetchFeed(ctx, feed.URL)
		if err != nil {
			log.Printf("Error fetching feed %s: %v", feed.URL, err)
			continue
		}

		// Update feed title if it has changed
		if content.Title != feed.Title {
			if err := u.store.UpdateFeedTitle(feed.ID, content.Title); err != nil {
				log.Printf("Error updating feed title: %v", err)
			}
		}

		// Add new posts
		for _, item := range content.Items {
			if err := u.store.AddPost(feed.ID, item.GUID, item.Title, item.Link, item.PublishedAt, item.Content); err != nil {
				log.Printf("Error adding post: %v", err)
			}
		}

		// Update last fetched timestamp
		if err := u.store.UpdateFeedLastFetched(feed.ID, time.Now()); err != nil {
			log.Printf("Error updating feed last fetched: %v", err)
		}
	}

	return nil
}
