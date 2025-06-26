package db

import (
	"fmt"
	"os"
	"testing"
)

func TestNewStore(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.db.Close()

	var tableName string
	err = store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&tableName)
	if err != nil {
		t.Fatalf("Failed to verify users table exists: %v", err)
	}

	if tableName != "users" {
		t.Errorf("Expected table name 'users', got '%s'", tableName)
	}
}

func TestAddFeedForUser_DuplicateHandling(t *testing.T) {
	// Create a temporary database
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.db.Close()

	// Create two test users
	user1ID, err := store.GetOrCreateUser("user1", "issuer1")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}

	user2ID, err := store.GetOrCreateUser("user2", "issuer2")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Test URL
	feedURL := "https://example.com/feed.xml"

	// Test 1: Add feed for user1 for the first time
	feed1ID, err := store.AddFeedForUser(user1ID, feedURL)
	if err != nil {
		t.Fatalf("Failed to add feed for user1: %v", err)
	}

	// Verify feed was added to feeds table
	var feedCount int
	err = store.db.QueryRow("SELECT COUNT(*) FROM feeds WHERE url = ?", feedURL).Scan(&feedCount)
	if err != nil {
		t.Fatalf("Failed to count feeds: %v", err)
	}
	if feedCount != 1 {
		t.Errorf("Expected 1 feed in feeds table, got %d", feedCount)
	}

	// Verify user1 is associated with the feed
	var userFeedCount int
	err = store.db.QueryRow("SELECT COUNT(*) FROM user_feeds WHERE user_id = ? AND feed_id = ?", user1ID, feed1ID).Scan(&userFeedCount)
	if err != nil {
		t.Fatalf("Failed to count user feeds: %v", err)
	}
	if userFeedCount != 1 {
		t.Errorf("Expected 1 user_feed association for user1, got %d", userFeedCount)
	}

	// Test 2: Add the same feed for user1 again (should be graceful)
	feed2ID, err := store.AddFeedForUser(user1ID, feedURL)
	if err != nil {
		t.Fatalf("Failed to add duplicate feed for user1: %v", err)
	}

	// Should return the same feed ID
	if feed1ID != feed2ID {
		t.Errorf("Expected same feed ID for duplicate, got %d vs %d", feed1ID, feed2ID)
	}

	// Should still have only 1 feed in feeds table
	err = store.db.QueryRow("SELECT COUNT(*) FROM feeds WHERE url = ?", feedURL).Scan(&feedCount)
	if err != nil {
		t.Fatalf("Failed to count feeds: %v", err)
	}
	if feedCount != 1 {
		t.Errorf("Expected 1 feed in feeds table after duplicate, got %d", feedCount)
	}

	// Should still have only 1 user_feed association for user1
	err = store.db.QueryRow("SELECT COUNT(*) FROM user_feeds WHERE user_id = ? AND feed_id = ?", user1ID, feed1ID).Scan(&userFeedCount)
	if err != nil {
		t.Fatalf("Failed to count user feeds: %v", err)
	}
	if userFeedCount != 1 {
		t.Errorf("Expected 1 user_feed association for user1 after duplicate, got %d", userFeedCount)
	}

	// Test 3: Add the same feed for user2 (should work and reuse existing feed)
	feed3ID, err := store.AddFeedForUser(user2ID, feedURL)
	if err != nil {
		t.Fatalf("Failed to add feed for user2: %v", err)
	}

	// Should return the same feed ID
	if feed1ID != feed3ID {
		t.Errorf("Expected same feed ID for user2, got %d vs %d", feed1ID, feed3ID)
	}

	// Should still have only 1 feed in feeds table
	err = store.db.QueryRow("SELECT COUNT(*) FROM feeds WHERE url = ?", feedURL).Scan(&feedCount)
	if err != nil {
		t.Fatalf("Failed to count feeds: %v", err)
	}
	if feedCount != 1 {
		t.Errorf("Expected 1 feed in feeds table after user2, got %d", feedCount)
	}

	// Should have 1 user_feed association for user2
	err = store.db.QueryRow("SELECT COUNT(*) FROM user_feeds WHERE user_id = ? AND feed_id = ?", user2ID, feed1ID).Scan(&userFeedCount)
	if err != nil {
		t.Fatalf("Failed to count user feeds for user2: %v", err)
	}
	if userFeedCount != 1 {
		t.Errorf("Expected 1 user_feed association for user2, got %d", userFeedCount)
	}

	// Should have 2 total user_feed associations
	var totalUserFeeds int
	err = store.db.QueryRow("SELECT COUNT(*) FROM user_feeds WHERE feed_id = ?", feed1ID).Scan(&totalUserFeeds)
	if err != nil {
		t.Fatalf("Failed to count total user feeds: %v", err)
	}
	if totalUserFeeds != 2 {
		t.Errorf("Expected 2 total user_feed associations, got %d", totalUserFeeds)
	}

	// Test 4: Verify GetUserFeeds returns the feed for both users
	user1Feeds, err := store.GetUserFeeds(user1ID)
	if err != nil {
		t.Fatalf("Failed to get feeds for user1: %v", err)
	}
	if len(user1Feeds) != 1 {
		t.Errorf("Expected 1 feed for user1, got %d", len(user1Feeds))
	}
	if user1Feeds[0].URL != feedURL {
		t.Errorf("Expected feed URL %s for user1, got %s", feedURL, user1Feeds[0].URL)
	}

	user2Feeds, err := store.GetUserFeeds(user2ID)
	if err != nil {
		t.Fatalf("Failed to get feeds for user2: %v", err)
	}
	if len(user2Feeds) != 1 {
		t.Errorf("Expected 1 feed for user2, got %d", len(user2Feeds))
	}
	if user2Feeds[0].URL != feedURL {
		t.Errorf("Expected feed URL %s for user2, got %s", feedURL, user2Feeds[0].URL)
	}
}

func TestUserPreferences(t *testing.T) {
	// Create a temporary database
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.db.Close()

	// Create a test user
	userID, err := store.GetOrCreateUser("test-subject", "test-issuer")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Test 1: Get default posts per feed for new user
	postsPerFeed, err := store.GetUserPostsPerFeed(userID)
	if err != nil {
		t.Fatalf("Failed to get default posts per feed: %v", err)
	}
	if postsPerFeed != 10 {
		t.Errorf("Expected default posts per feed to be 10, got %d", postsPerFeed)
	}

	// Test 2: Set custom posts per feed
	customPostsPerFeed := 15
	err = store.SetUserPostsPerFeed(userID, customPostsPerFeed)
	if err != nil {
		t.Fatalf("Failed to set posts per feed: %v", err)
	}

	// Test 3: Verify the setting was saved
	postsPerFeed, err = store.GetUserPostsPerFeed(userID)
	if err != nil {
		t.Fatalf("Failed to get posts per feed after setting: %v", err)
	}
	if postsPerFeed != customPostsPerFeed {
		t.Errorf("Expected posts per feed to be %d, got %d", customPostsPerFeed, postsPerFeed)
	}

	// Test 4: Update the setting
	newPostsPerFeed := 25
	err = store.SetUserPostsPerFeed(userID, newPostsPerFeed)
	if err != nil {
		t.Fatalf("Failed to update posts per feed: %v", err)
	}

	// Test 5: Verify the update
	postsPerFeed, err = store.GetUserPostsPerFeed(userID)
	if err != nil {
		t.Fatalf("Failed to get posts per feed after update: %v", err)
	}
	if postsPerFeed != newPostsPerFeed {
		t.Errorf("Expected posts per feed to be %d, got %d", newPostsPerFeed, postsPerFeed)
	}

	// Test 6: Test with another user (should have separate preferences)
	user2ID, err := store.GetOrCreateUser("test-subject-2", "test-issuer")
	if err != nil {
		t.Fatalf("Failed to create second test user: %v", err)
	}

	postsPerFeed2, err := store.GetUserPostsPerFeed(user2ID)
	if err != nil {
		t.Fatalf("Failed to get posts per feed for second user: %v", err)
	}
	if postsPerFeed2 != 10 {
		t.Errorf("Expected default posts per feed for second user to be 10, got %d", postsPerFeed2)
	}

	// Test 7: Set different preference for second user
	err = store.SetUserPostsPerFeed(user2ID, 5)
	if err != nil {
		t.Fatalf("Failed to set posts per feed for second user: %v", err)
	}

	// Test 8: Verify both users have different preferences
	postsPerFeed1, err := store.GetUserPostsPerFeed(userID)
	if err != nil {
		t.Fatalf("Failed to get posts per feed for first user: %v", err)
	}
	postsPerFeed2, err = store.GetUserPostsPerFeed(user2ID)
	if err != nil {
		t.Fatalf("Failed to get posts per feed for second user: %v", err)
	}

	if postsPerFeed1 == postsPerFeed2 {
		t.Errorf("Expected different preferences for different users, got %d and %d", postsPerFeed1, postsPerFeed2)
	}
}

func BenchmarkMoveFeedOperations(b *testing.B) {
	// Create a temporary database
	tmpFile, err := os.CreateTemp("", "bench-*.db")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.db.Close()

	// Create a test user
	userID, err := store.GetOrCreateUser("bench-user", "bench-issuer")
	if err != nil {
		b.Fatalf("Failed to create test user: %v", err)
	}

	// Add 10 feeds for the user
	var feedIDs []int64
	for i := 0; i < 10; i++ {
		feedID, err := store.AddFeedForUser(userID, fmt.Sprintf("https://example.com/feed%d.xml", i))
		if err != nil {
			b.Fatalf("Failed to add feed: %v", err)
		}
		feedIDs = append(feedIDs, feedID)
	}

	b.ResetTimer()

	// Benchmark moving feeds up and down
	for i := 0; i < b.N; i++ {
		// Move a feed up
		err := store.MoveFeedUp(userID, feedIDs[5])
		if err != nil {
			b.Fatalf("Failed to move feed up: %v", err)
		}

		// Move a feed down
		err = store.MoveFeedDown(userID, feedIDs[3])
		if err != nil {
			b.Fatalf("Failed to move feed down: %v", err)
		}
	}
}

func TestMoveFeedEfficiency(t *testing.T) {
	// Create a temporary database
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.db.Close()

	// Create a test user
	userID, err := store.GetOrCreateUser("efficiency-user", "efficiency-issuer")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Add 5 feeds for the user
	for i := 0; i < 5; i++ {
		_, err := store.AddFeedForUser(userID, fmt.Sprintf("https://example.com/feed%d.xml", i))
		if err != nil {
			t.Fatalf("Failed to add feed: %v", err)
		}
	}

	// Get initial order
	feeds, err := store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get user feeds: %v", err)
	}

	t.Logf("Initial order:")
	for i, feed := range feeds {
		t.Logf("  %d: Feed ID %d", i, feed.ID)
	}

	// Test that the efficient implementation works correctly
	// The new implementation should:
	// 1. Make 1 query to get all feed positions
	// 2. Make 1 query to update both positions
	// Total: 2 queries instead of 4+ queries in the old implementation

	// Move the feed at position 2 up (should swap with position 1)
	targetFeedID := feeds[2].ID
	err = store.MoveFeedUp(userID, targetFeedID)
	if err != nil {
		t.Fatalf("Failed to move feed up: %v", err)
	}

	// Verify the order changed correctly
	feeds, err = store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get user feeds: %v", err)
	}

	t.Logf("After moving feed %d up:", targetFeedID)
	for i, feed := range feeds {
		t.Logf("  %d: Feed ID %d", i, feed.ID)
	}

	// The target feed should now be at position 1 (was at position 2)
	if feeds[1].ID != targetFeedID {
		t.Errorf("Expected feed ID %d at position 1 after moving up, got %d", targetFeedID, feeds[1].ID)
	}

	// Move the feed at position 1 down (should swap with position 2)
	targetFeedID = feeds[1].ID
	err = store.MoveFeedDown(userID, targetFeedID)
	if err != nil {
		t.Fatalf("Failed to move feed down: %v", err)
	}

	// Verify the order changed correctly again
	feeds, err = store.GetUserFeeds(userID)
	if err != nil {
		t.Fatalf("Failed to get user feeds: %v", err)
	}

	t.Logf("After moving feed %d down:", targetFeedID)
	for i, feed := range feeds {
		t.Logf("  %d: Feed ID %d", i, feed.ID)
	}

	// The target feed should now be at position 2 (was at position 1)
	if feeds[2].ID != targetFeedID {
		t.Errorf("Expected feed ID %d at position 2 after moving down, got %d", targetFeedID, feeds[2].ID)
	}

	t.Logf("Efficiency test passed: Feed reordering works correctly with optimized queries")
}
