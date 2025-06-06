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