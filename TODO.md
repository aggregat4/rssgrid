# TODO

A prioritized list of improvements for RSSGrid. Items are grouped by area; each has enough context to scope the work and acceptance criteria to know when it's done. Implementation details are left to be figured out when tackling each item.

---

## 1. Authorization (security)

Several handlers trust the URL parameter and operate on resources without verifying that the signed-in user actually owns them. Any authenticated user can currently act on any other user's feeds/posts by guessing/enumerating IDs.

### Problems

- `handleDeleteFeed` (`internal/server/server.go`) calls `store.DeleteFeed(feedId)` with no user scoping — deletes the global feed row and cascades to every user subscribed to it.
- `handleGetPost` calls `store.GetPost(postID)` with no user check — any logged-in user can read any post in the DB.
- `handleMarkPostSeen` calls `store.MarkPostAsSeen(userID, postID)` — the store method blindly upserts into `user_post_states` for the given post, even if the post doesn't belong to one of the user's feeds.
- `handleMarkAllSeen` calls `store.MarkAllFeedPostsAsSeen(userID, feedID)` — no check that the user is subscribed to the feed.
- `handleMoveFeedUp` / `handleMoveFeedDown` are already user-scoped in the store (`WHERE user_id = ?`), so they are fine — but worth a test to lock that in.

### Acceptance criteria

- A user cannot delete a feed they aren't subscribed to (404/403, and the feed survives for other users).
- A user cannot read, mark seen, or "mark all seen" for posts/feeds that aren't theirs.
- Existing tests still pass; new tests in item 5 cover the cross-user cases.

---

## 2. Feed health surfacing

The updater (`internal/feed/updater.go`) logs fetch failures but the user never sees them, and broken feeds are retried every cycle with no backoff.

### Acceptance criteria

- A feed that 404s stops being retried every cycle after a few failures.
- The settings page shows which feeds are broken and why.
- After the feed recovers, the warning clears and the failure state resets.

---

## 3. Session / config hardening

`internal/server/server.go` hardcodes insecure session options and the config has no guard against placeholder/weak session keys.

### Acceptance criteria

- Starting the server with the example placeholder session key fails with a helpful message.
- `SecureCookies: true` produces `Secure` cookies; `false` (default) preserves current behavior for local dev.
- The misleading "7 days" comment on `MaxAge` (the value is actually 30 days) is gone.

---

## 4. Favicon + feed title fallback

Two small UX papercuts: no favicon is served (404s in browser logs), and feeds with an empty `<title>` render blank widget headers.

### Acceptance criteria

- `/favicon.svg` returns 200 with an image content type and no auth redirect; the browser tab shows an icon.
- A feed with an empty `<title>` no longer renders a blank widget header — the URL host (or full URL) is shown instead.

---

## 5. Tests

Build on the existing `internal/server/server_test.go`, `internal/db/store_test.go`, and `internal/feed/fetcher_test.go`. Focus on the gaps that items 1–4 open up.

### Acceptance criteria

- `./scripts/test.sh` passes with all new tests.
- `./scripts/lint.sh` stays clean.
- The authorization tests would fail against the current code (proving they actually cover the bug), and pass after item 1 is implemented.
- `mockStore` in `server_test.go` is kept in sync with whatever `StoreInterface` changes items 1–4 introduce.

---

## Notes / ordering

- Item 1 (authorization) should land first — it's a real bug class and item 5's tests depend on the new store interface it introduces.
- Item 2 (feed health) and item 4 (favicon/title fallback) are independent and can be done in parallel.
- Item 3 (config hardening) is independent but small; do it whenever.
- Item 5 (tests) should be done alongside each item rather than at the end, but it's listed last because its full scope is only known once 1–4 are tackled.
