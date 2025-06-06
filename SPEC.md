## **Revised Project Specification: RSSGrid - A Personal Feed Dashboard**

### **1. Project Overview & Philosophy**

**Name:** RSSGrid

**Mission:** To provide a simple, fast, and persistent personal dashboard for consuming text-based content from various web feeds. It serves as a replacement for services like Netvibes, focusing on minimalism, user control, and a "no-magic" technical approach.

**Core Philosophy:**
*   **Server-Centric:** The server does the heavy lifting: fetching feeds, rendering HTML, and managing state. The client should be as "dumb" as possible.
*   **Minimalism:** No JavaScript frameworks, no complex CSS pre-processors, no unnecessary features. The focus is on content consumption.
*   **Longevity:** By using standard, stable technologies (Go, SQLite, HTML), the application is designed to be easily maintained for a long time.
*   **User Ownership:** The user's configuration and data are central. All data is stored in a simple, self-contained database.

### **2. Core Concepts & Terminology**

*   **User:** An individual who logs into the system. Each user has their own unique set of feeds and dashboard configuration.
*   **Dashboard:** The main, and only, content page (`/`). It displays a grid of Widgets.
*   **Feed:** A source of content, identified by a URL (e.g., an RSS, Atom, or JSON Feed). Feeds are fetched and parsed by the server in the background.
*   **Post:** An individual item within a Feed (e.g., a blog post, a news article).
*   **Widget:** The visual representation of a single Feed on the User's Dashboard. It occupies a cell in the CSS Grid and displays the latest Posts from its associated Feed.
*   **Seen State:** A per-user, per-post flag indicating whether the User has viewed a particular Post. This must be visually distinct on the Dashboard.

### **3. Functional Requirements**

#### **3.1. User Authentication**
*   Authentication **MUST** be handled via OpenID Connect (OIDC).
*   The application will not have its own local username/password system.
*   If an unauthenticated user attempts to access any authenticated route (e.g., `/` or `/settings`), they MUST be redirected to the `/login` route.
*   The server must handle the OIDC callback, exchange the code for tokens, and validate the ID token.
*   Upon successful validation, the server will check if a `User` record exists for the given OIDC subject (`sub`) and issuer (`iss`). If not, a new `User` record is created.
*   A cryptographically secure session cookie will be set in the user's browser to maintain the logged-in state.
*   A "Logout" button must be available, which clears the session cookie and redirects to the login page.

#### **3.2. Dashboard View (`/`)**
*   This is the root URL and the primary interface of the application. It is accessible only to authenticated users.
*   The layout **MUST** be implemented using CSS Grid. The grid should be configurable, but for V1, a static 3-column grid is sufficient.
*   Each `Widget` on the Dashboard displays content from one `Feed`.
*   Each `Widget` **MUST** display:
    *   The `Feed` title.
    *   A list of the **10 most recent** `Posts`.
    *   Each `Post` in the list **MUST** display its title. The title should be a hyperlink to the original article (`<a href="..." target="_blank">`).
*   **Seen State Visualization:**
    *   `Posts` that have not been "seen" by the user must have a visually distinct style (e.g., normal font weight, a subtle light background color).
    *   `Posts` that have been "seen" must have a different style (e.g., slightly greyed-out text, `font-style: italic`).
    *   This styling **MUST** be achieved with CSS classes applied during server-side rendering (e.g., `<li class="post seen">...</li>`).
    *   When clicking on a post it should show the content for that post, this should be the raw sanitzid content we stored when fetching the feed.

#### **3.3. Marking Posts as Seen**
*   This is the primary user interaction on the Dashboard.
*   When a user clicks a `Post` link, that `Post` should be marked as "seen".
*   **Implementation:** This is a candidate for minimal, vanilla JavaScript.
    1.  Attach a `mousedown` or `click` event listener to all post links (`<a class="post-link">`). **The anchor tag for each post MUST include a `data-post-id` attribute containing the post's unique ID, e.g., `<a href="..." class="post-link" data-post-id="123">...</a>`.**
    2.  When triggered, the script will use the `fetch` API to send a `POST` request to a server endpoint (e.g., `/posts/{postid}/seen`) with the Post's ID and a boolean as payload indicating it was seen
    3.  This happens in the background. The user can still follow the link (`target="_blank"`) without interruption. The next time the Dashboard is rendered, the post will have the "seen" style.
*   A "Mark All as Read" button should be present at the top of each `Widget`. Clicking this will mark all currently visible posts in that `Widget` as seen and trigger a page reload to reflect the change. This MUST be a simple HTML form submission: `<form action="/feeds/mark-all-seen" method="POST">`. The form MUST include a hidden input field containing the feed's ID, e.g., `<input type="hidden" name="feed_id" value="42">`.

#### **3.4. Settings & Feed Management (`/settings`)**
*   A separate page, accessible only to authenticated users, for managing their dashboard.
*   **Functionality:**
    1.  **List Feeds:** Display a list of the user's currently subscribed `Feeds`. Each item in the list should have a "Remove" button (within a form).
    2.  **Add Feed:** A simple HTML form with a single text input for a new `Feed` URL. On submission, the server will validate the URL, attempt to fetch and parse it to ensure it's a valid feed, and if so, add it to the user's subscriptions. The page MUST re-render with clear user feedback: a success message if the feed was added, or an error message explaining the failure (e.g., "URL is not a valid feed") if it was not.
    3.  **Grid Placement (V1):** The order in which feeds are displayed is determined by the `grid_position` column. When a new feed is added, it MUST be assigned a `grid_position` equal to `MAX(grid_position) + 1` for that user. When a feed is removed, no re-indexing occurs. The dashboard rendering logic MUST simply order widgets by `grid_position` ASC.

#### **3.5. Background Feed Fetching**
*   The server **MUST** have a background process (a separate goroutine) that runs independently of user requests.
*   This process periodically fetches all unique `Feed` URLs stored in the database. It MUST run on a fixed interval, which defaults to 30 minutes.
*   For each `Feed`, it will parse the content.
*   For each `Post` in the parsed feed, it will check if a `Post` with the same GUID (or link, if GUID is absent) already exists in the database.
*   If the `Post` is new, it is inserted into the `posts` table.
*   The process must be resilient to errors (e.g., dead links, malformed XML, network timeouts) and should log them without crashing.
*   The feed checker should always check the HTTP caching headers: it should never refetch a feed before it is allowed to
*   The feed checker should have a configured timeout that is not too long so that long timeouts do  not block the process
*   All post contents must be run through the <https://github.com/microcosm-cc/bluemonday> Bluemonday sanitizer before storing in the database. The profile used should be the `bluemonday.UGCPolicy()` profile.

### **4. Technical Specification**

*   **Language:** Go (latest stable version)
*   **Database:** SQLite 3. The database file should be stored on the server's filesystem.
    * Use <github.com/aggregat4/go-baselib/migrations> for implementing the database schema with a simple migration library. This can look like:
    ```go
    var mymigrations = []migrations.Migration{
        {
            SequenceId: 1,
            Sql: `
            -- Enable WAL mode on the database to allow for concurrent reads and writes
            PRAGMA journal_mode=WAL;
            PRAGMA foreign_keys = ON;
            `,

        },
    }
    
    func (store *Store) InitAndVerifyDb(dbUrl string) error {
        var err error
        store.db, err = sql.Open("sqlite3", dbUrl)
        if err != nil {
            return fmt.Errorf("error opening database: %w", err)
        }
        return migrations.MigrateSchema(store.db, mymigrations)
    }
    ```
*   **Web Framework:** None. Use the standard library `net/http` package. For routing, a lightweight router like `chi` or `httprouter` is acceptable, but not required.
*   **Templating:** Use Go's standard `html/template` package for server-side rendering of all HTML pages.
*   **Frontend:**
    *   **CSS:** A single, simple vanilla CSS file. Layout **MUST** use CSS Grid.
    *   **JavaScript:** No JS frameworks. A single, small, vanilla JS file is permissible *only* for the "Mark as Seen" background `fetch` call. All other functionality must work without JavaScript.
*   **Dependencies (Go Libraries):**
    *   **OIDC:** `github.com/coreos/go-oidc/v3/oidc` for OIDC client functionality.
    *   **Session Management:** `github.com/alexedwards/scs`. 
    *   **Feed Parsing:** `github.com/mmcdole/gofeed` is an excellent choice for parsing RSS, Atom, and JSON feeds.
    *   **Database Driver:** `github.com/mattn/go-sqlite3` is the standard driver for SQLite.

#### **4.1. Configuration**
*   **The application MUST be configurable via environment variables.** This prevents hardcoding sensitive credentials and allows for flexible deployment.
*   **Required variables:**
    *   `MONOCLE_OIDC_ISSUER_URL`: The full URL of the OIDC provider.
    *   `MONOCLE_OIDC_CLIENT_ID`: The client ID provided by the OIDC provider.
    *   `MONOCLE_OIDC_CLIENT_SECRET`: The client secret provided by the OIDC provider.
    *   `MONOCLE_DB_PATH`: The file path for the SQLite database (e.g., `./monocle.db`).
    *   `MONOCLE_SESSION_KEY`: A cryptographically secure secret key for signing session cookies.

### **5. Data Model (SQLite Schema)**

```sql
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
-- This allows multiple users to subscribe to the same feed.
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
```

### **6. API Endpoints / Routes**

| Method | Path                        | Authentication | Description                                                                    |
| :----- | :-------------------------- | :------------- | :----------------------------------------------------------------------------- |
| `GET`  | `/`                         | Required       | Renders the main Dashboard grid. Redirects to `/login` if not authenticated.   |
| `GET`  | `/auth/callback`            | Not Required   | Handles the callback from the OIDC provider. Creates session on success.       |
| `POST` | `/logout`                   | Required       | Clears the user's session cookie and logs them out via a form submission.      |
| `GET`  | `/settings`                 | Required       | Renders the settings page for managing feeds.                                  |
| `POST` | `/settings/feeds`       | Required       | Handles the form submission for adding a new feed URL. Expects at the feed `url` as a payload.          |
| `POST` | `/settings/feeds/{feed_id}/delete`    | Required       | Handles form submission for removing a feed.            |
| `POST` | `/posts/{post_id}/seen`      | Required       | **(JS fetch)** API endpoint to mark a single post as seen. |
| `POST` | `/feeds/{feed_id}/seen`      | Required       | **(Form Submit)** Marks all posts for a feed as seen.   |

