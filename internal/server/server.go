package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"runtime/debug"
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
	GetOrCreateUser(subject, issuer string) (int64, error)
	AddFeed(url string) (int64, error)
	AddFeedForUser(userID int64, url string) (int64, error)
	UpdateFeedTitle(feedID int64, title string) error
	AddPost(feedID int64, guid, title, link string, publishedAt time.Time, content string) error
	DeleteFeed(feedID string) error
	MarkPostAsSeen(userID int64, postID string) error
	MarkAllFeedPostsAsSeen(userID int64, feedID string) error
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
	templates, err := templates.LoadTemplates()
	if err != nil {
		log.Printf("Error loading templates: %v\nStack trace:\n%s", err, debug.Stack())
		return nil, fmt.Errorf("error loading templates: %w", err)
	}

	// Validate that required templates exist
	requiredTemplates := []string{"dashboard.html", "settings.html"}
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
		r.Post("/settings/feeds", s.handleAddFeed)
		r.Post("/settings/feeds/{feedId}/delete", s.handleDeleteFeed)
		r.Post("/posts/{postId}/seen", s.handleMarkPostSeen)
		r.Post("/feeds/{feedId}/seen", s.handleMarkAllSeen)
	})

	log.Printf("Starting server on %s", addr)
	return http.ListenAndServe(addr, r)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userId := s.getUserID(r)

	feeds, err := s.store.GetUserFeeds(userId)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching feeds", "Error fetching feeds for user", err, "userId", userId)
		return
	}

	type FeedData struct {
		Feed  db.Feed
		Posts []db.Post
	}

	var feedData []FeedData
	for _, f := range feeds {
		posts, err := s.store.GetFeedPosts(f.ID, userId, 10)
		if err != nil {
			s.logErrorAndRespond(w, http.StatusInternalServerError, "Error fetching posts", "Error fetching posts for feed", err, "feedId", f.ID, "userId", userId)
			return
		}
		feedData = append(feedData, FeedData{Feed: f, Posts: posts})
	}

	data := struct {
		Feeds []FeedData
	}{
		Feeds: feedData,
	}

	log.Printf("Rendering dashboard template with %d feeds", len(feedData))
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

	// Get flash messages
	session, err := s.sessions.Get(r, "user_session")
	var flashMessages []map[string]string
	if err == nil {
		flashes := session.Flashes("success", "error")
		for _, flash := range flashes {
			if flashMap, ok := flash.(map[string]string); ok {
				flashMessages = append(flashMessages, flashMap)
			} else if flashStr, ok := flash.(string); ok {
				flashMessages = append(flashMessages, map[string]string{"message": flashStr, "type": "success"})
			}
		}
		session.Save(r, w)
	}

	data := struct {
		Feeds         []db.Feed
		FlashMessages []map[string]string
	}{
		Feeds:         feeds,
		FlashMessages: flashMessages,
	}

	log.Printf("Rendering settings template with %d feeds", len(feeds))
	if err := s.templates.ExecuteTemplate(w, "settings.html", data); err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error rendering template", "Error rendering settings template", err, "templateData", data)
		return
	}
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userId := s.getUserID(r)
	if userId == 0 {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	content, err := s.fetcher.FetchFeed(r.Context(), url)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusBadRequest, "Invalid feed URL", "Error fetching feed from URL", err, "url", url)
		return
	}

	feedId, err := s.store.AddFeedForUser(userId, url)
	if err != nil {
		s.logErrorAndRespond(w, http.StatusInternalServerError, "Error adding feed", "Error adding feed with URL", err, "url", url)
		return
	}

	// Update feed title
	if content.Title != "" {
		if err := s.store.UpdateFeedTitle(feedId, content.Title); err != nil {
			s.logErrorAndRespond(w, http.StatusInternalServerError, "Error updating feed title", "Error updating feed title for feed", err, "feedId", feedId)
			return
		}
	}

	// Add posts
	for _, item := range content.Items {
		if err := s.store.AddPost(feedId, item.GUID, item.Title, item.Link, item.PublishedAt, item.Content); err != nil {
			s.logErrorAndRespond(w, http.StatusInternalServerError, "Error adding post", "Error adding post with GUID to feed", err, "guid", item.GUID, "feedId", feedId)
			return
		}
	}

	// Set a success message in the session
	session, err := s.sessions.Get(r, "user_session")
	if err == nil {
		session.AddFlash("Feed added successfully!", "success")
		session.Save(r, w)
	}

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
