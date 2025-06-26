package server

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	baseliboidc "github.com/aggregat4/go-baselib-services/v3/oidc"
	"github.com/aggregat4/rssgrid/internal/db"
	"github.com/aggregat4/rssgrid/internal/feed"
	"github.com/aggregat4/rssgrid/internal/templates"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
)

type Server struct {
	store      StoreInterface
	sessions   *sessions.CookieStore
	fetcher    *feed.Fetcher
	templates  *template.Template
	oidcConfig *baseliboidc.OidcConfiguration
}

// StoreInterface defines the interface that the server needs
type StoreInterface interface {
	GetUserFeeds(userID int64) ([]db.Feed, error)
	GetFeedPosts(feedID, userID int64, limit int) ([]db.Post, error)
	GetPost(postID int64) (*db.Post, error)
	GetOrCreateUser(subject, issuer string) (int64, error)
	AddFeed(url string) (int64, error)
	AddFeedForUser(userID int64, url string) (int64, error)
	UpdateFeedTitle(feedID int64, title string) error
	AddPost(feedID int64, guid, title, link string, publishedAt time.Time, content string) error
	DeleteFeed(feedID string) error
	MarkPostAsSeen(userID int64, postID string) error
	MarkAllFeedPostsAsSeen(userID int64, feedID string) error
	GetUserPostsPerFeed(userID int64) (int, error)
	SetUserPostsPerFeed(userID int64, postsPerFeed int) error
	MoveFeedUp(userID int64, feedID int64) error
	MoveFeedDown(userID int64, feedID int64) error
	GetUserColumns(userID int64) (int, error)
	SetUserColumns(userID int64, columns int) error
}

type FlashMessage struct {
	Message string
	Type    string
}

// addFlashMessage adds a flash message to the session
func (s *Server) addFlashMessage(w http.ResponseWriter, r *http.Request, message, flashType string) {
	session, err := s.sessions.Get(r, "user_session")
	if err != nil {
		log.Printf("Error getting session for flash message: %v", err)
		return
	}

	session.AddFlash(message, flashType)
	if err := session.Save(r, w); err != nil {
		log.Printf("Error saving session with flash message: %v\nStack trace:\n%s", err, debug.Stack())
	}
}

// addErrorFlash adds an error flash message to the session
func (s *Server) addErrorFlash(w http.ResponseWriter, r *http.Request, message string) {
	s.addFlashMessage(w, r, message, "error")
}

// addSuccessFlash adds a success flash message to the session
func (s *Server) addSuccessFlash(w http.ResponseWriter, r *http.Request, message string) {
	s.addFlashMessage(w, r, message, "success")
}

// getFlashMessages retrieves all flash messages from the session
func (s *Server) getFlashMessages(w http.ResponseWriter, r *http.Request) []FlashMessage {
	session, err := s.sessions.Get(r, "user_session")
	var flashMessages []FlashMessage
	if err != nil {
		log.Printf("Error getting session for flash messages: %v", err)
		return flashMessages
	}

	// Get error flash messages
	flashes := session.Flashes("error")
	for _, flash := range flashes {
		flashMessages = append(flashMessages, FlashMessage{Message: flash.(string), Type: "error"})
	}

	// Get success flash messages
	flashes = session.Flashes("success")
	for _, flash := range flashes {
		flashMessages = append(flashMessages, FlashMessage{Message: flash.(string), Type: "success"})
	}

	// Save the session after consuming flash messages to remove them from the session
	if err := session.Save(r, w); err != nil {
		log.Printf("Error saving session: %v\nStack trace:\n%s", err, debug.Stack())
	}

	return flashMessages
}

// getUserID extracts the user ID from the session
func (s *Server) getUserID(r *http.Request) int64 {
	session, err := s.sessions.Get(r, "user_session")
	if err != nil {
		log.Printf("Error getting session: %v\nStack trace:\n%s", err, debug.Stack())
		return 0
	}
	userID, ok := session.Values["user_id"].(int64)
	if !ok {
		log.Printf("Error: user_id not found in session or wrong type\nStack trace:\n%s", debug.Stack())
		return 0
	}
	return userID
}

func NewServer(store StoreInterface, oidcConfig *baseliboidc.OidcConfiguration, sessionKey string) (*Server, error) {
	sessionStore := sessions.NewCookieStore([]byte(sessionKey))

	// Configure session store options to ensure flash messages persist
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30, // 7 days
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	}

	templates, err := templates.LoadTemplates()
	if err != nil {
		log.Printf("Error loading templates: %v\nStack trace:\n%s", err, debug.Stack())
		return nil, fmt.Errorf("error loading templates: %w", err)
	}

	// Validate that required templates exist
	requiredTemplates := []string{"dashboard.html", "settings.html", "post.html"}
	for _, tmplName := range requiredTemplates {
		if tmpl := templates.Lookup(tmplName); tmpl == nil {
			log.Printf("Warning: Required template '%s' not found", tmplName)
		} else {
			log.Printf("Template '%s' loaded successfully", tmplName)
		}
	}

	log.Printf("Successfully loaded templates")

	// Create fetcher only if store is a concrete db.Store type
	var fetcher *feed.Fetcher
	if concreteStore, ok := store.(*db.Store); ok {
		fetcher = feed.NewFetcher(concreteStore)
	}

	return &Server{
		store:      store,
		sessions:   sessionStore,
		fetcher:    fetcher,
		templates:  templates,
		oidcConfig: oidcConfig,
	}, nil
}

// panicRecoveryMiddleware is a custom middleware that logs panics with full stack traces
func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v\nStack trace:\n%s", err, debug.Stack())
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logErrorAndRespond logs an error with stack trace and context, then sends an HTTP error response
func (s *Server) logErrorAndRespond(w http.ResponseWriter, statusCode int, userMessage, logMessage string, err error, context ...interface{}) {
	log.Printf("%s: %v\nContext: %v\nStack trace:\n%s", logMessage, err, context, debug.Stack())
	http.Error(w, userMessage, statusCode)
}

func (s *Server) Start(addr string) error {
	return s.StartWithContext(context.Background(), addr)
}

func (s *Server) StartWithContext(ctx context.Context, addr string) error {
	oidcAuthenticationMiddleware := s.oidcConfig.CreateOidcAuthenticationMiddleware(
		func(r *http.Request) bool {
			session, err := s.sessions.Get(r, "user_session")
			if err != nil {
				log.Printf("Error getting session in auth middleware: %v\nStack trace:\n%s", err, debug.Stack())
				return false
			}
			return session.Values["user_id"] != nil
		},
		func(r *http.Request) bool {
			return r.URL.Path == "/auth/callback"
		},
	)

	oidcCallbackHandler := s.oidcConfig.CreateOidcCallbackHandler(
		baseliboidc.CreateSTDSessionBasedOidcDelegate(
			func(w http.ResponseWriter, r *http.Request, idToken *oidc.IDToken) error {
				userId, err := s.store.GetOrCreateUser(idToken.Subject, idToken.Issuer)
				if err != nil {
					log.Printf("Error getting or creating user for subject %s, issuer %s: %v\nStack trace:\n%s",
						idToken.Subject, idToken.Issuer, err, debug.Stack())
					return fmt.Errorf("error getting or creating user: %w", err)
				}
				session, err := s.sessions.Get(r, "user_session")
				if err != nil {
					log.Printf("Error getting session for user %d: %v\nStack trace:\n%s", userId, err, debug.Stack())
					return fmt.Errorf("error getting session: %w", err)
				}
				session.Values["user_id"] = userId
				if err := session.Save(r, w); err != nil {
					log.Printf("Error saving session for user %d: %v\nStack trace:\n%s", userId, err, debug.Stack())
					return fmt.Errorf("error saving session: %w", err)
				}
				return nil
			},
			"/",
		),
	)

	r := chi.NewRouter()

	// Middleware
	r.Use(panicRecoveryMiddleware) // Add our custom panic recovery first
	r.Use(oidcAuthenticationMiddleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public routes
	r.Get("/auth/callback", oidcCallbackHandler)

	// Static files
	fileServer := templates.CreateStaticFileServer()
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Get("/", s.handleDashboard)
		r.Get("/settings", s.handleSettings)
		r.Get("/posts/{postId}", s.handleGetPost)
		r.Post("/logout", s.handleLogout)
		r.Post("/settings/feeds", s.handleAddFeed)
		r.Post("/settings/feeds/{feedId}/delete", s.handleDeleteFeed)
		r.Post("/settings/preferences", s.handleUpdatePreferences)
		r.Post("/settings/feeds/{feedId}/move-up", s.handleMoveFeedUp)
		r.Post("/settings/feeds/{feedId}/move-down", s.handleMoveFeedDown)
		r.Post("/posts/{postId}/seen", s.handleMarkPostSeen)
		r.Post("/feeds/{feedId}/seen", s.handleMarkAllSeen)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	log.Printf("Starting server on %s", addr)

	// Start server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	log.Printf("Shutting down HTTP server...")

	// Create a context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
		return err
	}

	log.Printf("HTTP server shutdown complete")
	return nil
}

func splitFeedsIntoColumns[T any](feeds []T, numCols int) [][]T {
	if numCols < 1 {
		numCols = 1
	}
	columns := make([][]T, numCols)
	for i, feed := range feeds {
		col := i % numCols
		columns[col] = append(columns[col], feed)
	}
	return columns
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userId := s.getUserID(r)

	feeds, err := s.store.GetUserFeeds(userId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching feeds", "Error fetching feeds for user", err, "userId", userId)
		return
	}

	// Get user's posts per feed preference
	postsPerFeed, err := s.store.GetUserPostsPerFeed(userId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching user preferences", "Error fetching posts per feed preference", err, "userId", userId)
		return
	}

	// Get user's column preference
	columns, err := s.store.GetUserColumns(userId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching user preferences", "Error fetching columns preference", err, "userId", userId)
		return
	}

	type FeedData struct {
		Feed  db.Feed
		Posts []db.Post
	}

	var feedData []FeedData
	for _, f := range feeds {
		posts, err := s.store.GetFeedPosts(f.ID, userId, postsPerFeed)
		if err != nil {
			s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching posts", "Error fetching posts for feed", err, "feedId", f.ID, "userId", userId)
			return
		}
		feedData = append(feedData, FeedData{Feed: f, Posts: posts})
	}

	columnsData := splitFeedsIntoColumns(feedData, columns)

	data := struct {
		Columns     [][]FeedData
		ColumnCount int
	}{
		Columns:     columnsData,
		ColumnCount: columns,
	}

	log.Printf("Rendering dashboard template with %d feeds in %d columns", len(feedData), columns)
	if err := s.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error rendering template", "Error rendering dashboard template", err, "templateData", data)
		return
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	userId := s.getUserID(r)

	feeds, err := s.store.GetUserFeeds(userId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching feeds", "Error fetching feeds for user", err, "userId", userId)
		return
	}

	// Get user's posts per feed preference
	postsPerFeed, err := s.store.GetUserPostsPerFeed(userId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching user preferences", "Error fetching posts per feed preference", err, "userId", userId)
		return
	}

	// Get user's column preference
	columns, err := s.store.GetUserColumns(userId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching user preferences", "Error fetching columns preference", err, "userId", userId)
		return
	}

	// Get flash messages
	flashMessages := s.getFlashMessages(w, r)

	data := struct {
		Feeds         []db.Feed
		FlashMessages []FlashMessage
		PostsPerFeed  int
		Columns       int
	}{
		Feeds:         feeds,
		FlashMessages: flashMessages,
		PostsPerFeed:  postsPerFeed,
		Columns:       columns,
	}

	log.Printf("Rendering settings template with %d feeds", len(feeds))
	if err := s.templates.ExecuteTemplate(w, "settings.html", data); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error rendering template", "Error rendering settings template", err, "templateData", data)
		return
	}
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	userId := s.getUserID(r)
	if userId == 0 {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		// Set error message and redirect
		s.addErrorFlash(w, r, "URL is required")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	content, err := s.fetcher.FetchFeed(r.Context(), url)
	if err != nil {
		// Log the error for debugging
		log.Printf("Error fetching feed from URL: %v\nContext: [url %s]\nStack trace:\n%s", err, url, debug.Stack())

		// Set error message and redirect
		s.addErrorFlash(w, r, "Invalid feed URL or unable to fetch feed")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	feedId, err := s.store.AddFeedForUser(userId, url)
	if err != nil {
		// Log the error for debugging
		log.Printf("Error adding feed with URL: %v\nContext: [url %s]\nStack trace:\n%s", err, url, debug.Stack())

		// Set error message and redirect
		s.addErrorFlash(w, r, "Error adding feed. Please try again.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Update feed title
	if content.Title != "" {
		if err := s.store.UpdateFeedTitle(feedId, content.Title); err != nil {
			log.Printf("Error updating feed title for feed: %v\nContext: [feedId %d]\nStack trace:\n%s", err, feedId, debug.Stack())
			// Don't fail the entire operation for title update errors
		}
	}

	// Add posts
	for _, item := range content.Items {
		if err := s.store.AddPost(feedId, item.GUID, item.Title, item.Link, item.PublishedAt, item.Content); err != nil {
			log.Printf("Error adding post with GUID to feed: %v\nContext: [guid %s, feedId %d]\nStack trace:\n%s", err, item.GUID, feedId, debug.Stack())
			// Continue adding other posts even if one fails
		}
	}

	// Set a success message in the session
	s.addSuccessFlash(w, r, "Feed added successfully!")

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	feedId := chi.URLParam(r, "feedId")
	if feedId == "" {
		http.Error(w, "Invalid feed ID", http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteFeed(feedId); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error deleting feed", "Error deleting feed", err, "feedId", feedId)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleMarkPostSeen(w http.ResponseWriter, r *http.Request) {
	postId := chi.URLParam(r, "postId")
	if postId == "" {
		http.Error(w, "Invalid post ID", http.StatusBadRequest)
		return
	}

	userId := s.getUserID(r)

	if err := s.store.MarkPostAsSeen(userId, postId); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error marking post as seen", "Error marking post as seen for user", err, "postId", postId, "userId", userId)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMarkAllSeen(w http.ResponseWriter, r *http.Request) {
	feedId := chi.URLParam(r, "feedId")
	if feedId == "" {
		http.Error(w, "Invalid feed ID", http.StatusBadRequest)
		return
	}

	userId := s.getUserID(r)

	if err := s.store.MarkAllFeedPostsAsSeen(userId, feedId); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error marking all posts as seen", "Error marking all posts as seen for feed", err, "feedId", feedId, "userId", userId)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userId := s.getUserID(r)
	if userId == 0 {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	postsPerFeed, err := strconv.Atoi(r.FormValue("postsPerFeed"))
	if err != nil {
		s.logErrorAndRespond(w, http.StatusBadRequest, "Invalid posts per feed format", "Error parsing posts per feed", err)
		return
	}

	columns, err := strconv.Atoi(r.FormValue("columns"))
	if err != nil || columns < 1 {
		s.logErrorAndRespond(w, http.StatusBadRequest, "Invalid columns format", "Error parsing columns", err)
		return
	}

	if err := s.store.SetUserPostsPerFeed(userId, postsPerFeed); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error updating posts per feed", "Error updating posts per feed for user", err, "userId", userId, "postsPerFeed", postsPerFeed)
		return
	}
	if err := s.store.SetUserColumns(userId, columns); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error updating columns", "Error updating columns for user", err, "userId", userId, "columns", columns)
		return
	}

	// Set a success message in the session
	s.addSuccessFlash(w, r, "Preferences updated successfully!")

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	postIdStr := chi.URLParam(r, "postId")
	if postIdStr == "" {
		http.Error(w, "Invalid post ID", http.StatusBadRequest)
		return
	}

	postId, err := strconv.ParseInt(postIdStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid post ID format", http.StatusBadRequest)
		return
	}

	post, err := s.store.GetPost(postId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching post", "Error fetching post", err, "postId", postId)
		return
	}

	data := struct {
		Post struct {
			ID          int64
			Title       string
			Link        string
			PublishedAt time.Time
			Content     template.HTML
		}
	}{
		Post: struct {
			ID          int64
			Title       string
			Link        string
			PublishedAt time.Time
			Content     template.HTML
		}{
			ID:          post.ID,
			Title:       post.Title,
			Link:        post.Link,
			PublishedAt: post.PublishedAt,
			Content:     template.HTML(post.Content),
		},
	}

	log.Printf("Rendering post template with post ID %d", postId)
	if err := s.templates.ExecuteTemplate(w, "post.html", data); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error rendering template", "Error rendering post template", err, "postId", postId)
		return
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	session, err := s.sessions.Get(r, "user_session")
	if err != nil {
		log.Printf("Error getting session for logout: %v\nStack trace:\n%s", err, debug.Stack())
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session.Values["user_id"] = nil
	if err := session.Save(r, w); err != nil {
		log.Printf("Error saving session for logout: %v\nStack trace:\n%s", err, debug.Stack())
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleMoveFeedUp(w http.ResponseWriter, r *http.Request) {
	feedIdStr := chi.URLParam(r, "feedId")
	if feedIdStr == "" {
		http.Error(w, "Invalid feed ID", http.StatusBadRequest)
		return
	}

	feedId, err := strconv.ParseInt(feedIdStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid feed ID format", http.StatusBadRequest)
		return
	}

	userId := s.getUserID(r)

	if err := s.store.MoveFeedUp(userId, feedId); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error moving feed up", "Error moving feed up for user", err, "feedId", feedId, "userId", userId)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleMoveFeedDown(w http.ResponseWriter, r *http.Request) {
	feedIdStr := chi.URLParam(r, "feedId")
	if feedIdStr == "" {
		http.Error(w, "Invalid feed ID", http.StatusBadRequest)
		return
	}

	feedId, err := strconv.ParseInt(feedIdStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid feed ID format", http.StatusBadRequest)
		return
	}

	userId := s.getUserID(r)

	if err := s.store.MoveFeedDown(userId, feedId); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error moving feed down", "Error moving feed down for user", err, "feedId", feedId, "userId", userId)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
