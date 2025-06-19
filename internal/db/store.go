package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/aggregat4/go-baselib/migrations"
	_ "github.com/mattn/go-sqlite3"
	"github.com/microcosm-cc/bluemonday"
)

var mymigrations = []migrations.Migration{
	{
		SequenceId: 1,
		Sql: `
-- Enable WAL mode on the database to allow for concurrent reads and writes
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys = ON;

-- Stores user information, linked to their OIDC identity.
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    oidc_subject TEXT NOT NULL, -- The 'sub' claim from the OIDC token
    oidc_issuer TEXT NOT NULL,  -- The 'iss' claim from the OIDC token
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(oidc_subject, oidc_issuer)
);

-- Stores the master list of all feed sources.
CREATE TABLE feeds (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,          -- The unique URL of the feed
    title TEXT,                        -- The title of the feed, fetched from the feed itself
    last_fetched_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- A junction table to link users to the feeds they subscribe to.
CREATE TABLE user_feeds (
    user_id INTEGER NOT NULL,
    feed_id INTEGER NOT NULL,
    grid_position INTEGER DEFAULT 0, -- For simple ordering
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(feed_id) REFERENCES feeds(id) ON DELETE CASCADE,
    PRIMARY KEY(user_id, feed_id)
);

-- Stores individual posts/articles from all feeds.
CREATE TABLE posts (
    id INTEGER PRIMARY KEY,
    feed_id INTEGER NOT NULL,
    guid TEXT NOT NULL,          -- Unique identifier from the feed (guid, id, or link)
    title TEXT,
    link TEXT NOT NULL,
    published_at DATETIME,
    content TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(feed_id) REFERENCES feeds(id) ON DELETE CASCADE,
    UNIQUE(feed_id, guid)
);

-- Stores the "seen" state for each user and each post.
CREATE TABLE user_post_states (
    user_id INTEGER NOT NULL,
    post_id INTEGER NOT NULL,
    seen INTEGER NOT NULL DEFAULT 0, -- Using INTEGER 0 for false, 1 for true
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE,
    PRIMARY KEY(user_id, post_id)
);
`,
	},
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	store := &Store{}
	if err := store.InitAndVerifyDb(dbPath); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *Store) InitAndVerifyDb(dbPath string) error {
	var err error
	store.db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("error opening database: %w", err)
	}
	return migrations.MigrateSchema(store.db, mymigrations)
}

// User related methods
func (store *Store) GetOrCreateUser(oidcSubject, oidcIssuer string) (int64, error) {
	var userId int64
	err := store.db.QueryRow(
		"SELECT id FROM users WHERE oidc_subject = ? AND oidc_issuer = ?",
		oidcSubject, oidcIssuer,
	).Scan(&userId)

	if err == sql.ErrNoRows {
		result, err := store.db.Exec(
			"INSERT INTO users (oidc_subject, oidc_issuer) VALUES (?, ?)",
			oidcSubject, oidcIssuer,
		)
		if err != nil {
			return 0, fmt.Errorf("error creating user: %w", err)
		}
		userId, err = result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("error getting last insert id: %w", err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("error querying user: %w", err)
	}

	return userId, nil
}

// Feed related methods
func (store *Store) AddFeed(url string) (int64, error) {
	result, err := store.db.Exec(
		"INSERT INTO feeds (url) VALUES (?)",
		url,
	)
	if err != nil {
		return 0, fmt.Errorf("error adding feed: %w", err)
	}
	return result.LastInsertId()
}

func (store *Store) GetUserFeeds(userId int64) ([]Feed, error) {
	rows, err := store.db.Query(`
		SELECT f.id, f.url, f.title, f.last_fetched_at, uf.grid_position
		FROM feeds f
		JOIN user_feeds uf ON f.id = uf.feed_id
		WHERE uf.user_id = ?
		ORDER BY uf.grid_position ASC
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying user feeds: %w", err)
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		var lastFetched sql.NullTime
		err := rows.Scan(&f.ID, &f.URL, &f.Title, &lastFetched, &f.GridPosition)
		if err != nil {
			return nil, fmt.Errorf("error scanning feed: %w", err)
		}
		if lastFetched.Valid {
			f.LastFetchedAt = lastFetched.Time
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

// Feed represents a feed in the database
type Feed struct {
	ID            int64
	URL           string
	Title         string
	LastFetchedAt time.Time
	GridPosition  int
}

// Post related methods
func (store *Store) AddPost(feedId int64, guid, title, link string, publishedAt time.Time, content string) error {
	// Sanitize content before storing
	sanitizedContent := bluemonday.UGCPolicy().Sanitize(content)
	_, err := store.db.Exec(`
		INSERT OR IGNORE INTO posts (feed_id, guid, title, link, published_at, content)
		VALUES (?, ?, ?, ?, ?, ?)
	`, feedId, guid, title, link, publishedAt, sanitizedContent)
	if err != nil {
		return fmt.Errorf("error adding post: %w", err)
	}
	return nil
}

func (store *Store) GetFeedPosts(feedId int64, userId int64, limit int) ([]Post, error) {
	rows, err := store.db.Query(`
		SELECT p.id, p.title, p.link, p.published_at, p.content,
		       COALESCE(ups.seen, 0) as seen
		FROM posts p
		LEFT JOIN user_post_states ups ON p.id = ups.post_id AND ups.user_id = ?
		WHERE p.feed_id = ?
		ORDER BY p.published_at DESC
		LIMIT ?
	`, userId, feedId, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying feed posts: %w", err)
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		err := rows.Scan(&p.ID, &p.Title, &p.Link, &p.PublishedAt, &p.Content, &p.Seen)
		if err != nil {
			return nil, fmt.Errorf("error scanning post: %w", err)
		}
		posts = append(posts, p)
	}
	return posts, nil
}

type Post struct {
	ID          int64
	Title       string
	Link        string
	PublishedAt time.Time
	Content     string
	Seen        bool
}

func (store *Store) MarkPostAsSeen(userId int64, postId string) error {
	_, err := store.db.Exec(`
		INSERT INTO user_post_states (user_id, post_id, seen)
		VALUES (?, ?, 1)
		ON CONFLICT(user_id, post_id) DO UPDATE SET seen = 1
	`, userId, postId)
	if err != nil {
		return fmt.Errorf("error marking post as seen: %w", err)
	}
	return nil
}

func (store *Store) MarkAllFeedPostsAsSeen(userId int64, feedId string) error {
	_, err := store.db.Exec(`
		INSERT INTO user_post_states (user_id, post_id, seen)
		SELECT ?, p.id, 1
		FROM posts p
		WHERE p.feed_id = ?
		ON CONFLICT(user_id, post_id) DO UPDATE SET seen = 1
	`, userId, feedId)
	if err != nil {
		return fmt.Errorf("error marking all feed posts as seen: %w", err)
	}
	return nil
}

func (store *Store) GetAllFeeds() ([]Feed, error) {
	rows, err := store.db.Query(`
		SELECT id, url, title, last_fetched_at
		FROM feeds
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying feeds: %w", err)
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		var lastFetched sql.NullTime
		err := rows.Scan(&f.ID, &f.URL, &f.Title, &lastFetched)
		if err != nil {
			return nil, fmt.Errorf("error scanning feed: %w", err)
		}
		if lastFetched.Valid {
			f.LastFetchedAt = lastFetched.Time
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
}

func (store *Store) UpdateFeedTitle(feedId int64, title string) error {
	_, err := store.db.Exec(`
		UPDATE feeds
		SET title = ?
		WHERE id = ?
	`, title, feedId)
	if err != nil {
		return fmt.Errorf("error updating feed title: %w", err)
	}
	return nil
}

func (store *Store) UpdateFeedLastFetched(feedId int64, timestamp time.Time) error {
	_, err := store.db.Exec(`
		UPDATE feeds
		SET last_fetched_at = ?
		WHERE id = ?
	`, timestamp, feedId)
	if err != nil {
		return fmt.Errorf("error updating feed last fetched: %w", err)
	}
	return nil
}

func (store *Store) DeleteFeed(feedId string) error {
	_, err := store.db.Exec(`
		DELETE FROM feeds
		WHERE id = ?
	`, feedId)
	if err != nil {
		return fmt.Errorf("error deleting feed: %w", err)
	}
	return nil
}
