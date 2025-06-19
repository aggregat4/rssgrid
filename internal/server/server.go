package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	baseliboidc "github.com/aggregat4/go-baselib-services/v3/oidc"
	"github.com/boris/go-rssgrid/internal/db"
	"github.com/boris/go-rssgrid/internal/feed"
	"github.com/boris/go-rssgrid/internal/templates"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"
)

type Server struct {
	store      *db.Store
	sessions   *sessions.CookieStore
	fetcher    *feed.Fetcher
	templates  *template.Template
	oidcConfig *baseliboidc.OidcConfiguration
}

// getUserID extracts the user ID from the session
func (s *Server) getUserID(r *http.Request) int64 {
	session, _ := s.sessions.Get(r, "user_session")
	return session.Values["user_id"].(int64)
}

func NewServer(store *db.Store, oidcConfig *baseliboidc.OidcConfiguration, sessionKey string) (*Server, error) {
	// Initialize session store
	sessionStore := sessions.NewCookieStore([]byte(sessionKey))
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60, // 30 days
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}

	// Load templates from embedded filesystem
	templates, err := templates.LoadTemplates()
	if err != nil {
		return nil, fmt.Errorf("error loading templates: %w", err)
	}
	return &Server{
		store:      store,
		sessions:   sessionStore,
		fetcher:    feed.NewFetcher(),
		templates:  templates,
		oidcConfig: oidcConfig,
	}, nil
}

func (s *Server) Start(addr string) error {
	oidcAuthenticationMiddleware := s.oidcConfig.CreateOidcAuthenticationMiddleware(
		func(r *http.Request) bool {
			session, _ := s.sessions.Get(r, "user_session")
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
					return fmt.Errorf("error getting or creating user: %w", err)
				}
				session, _ := s.sessions.Get(r, "user_session")
				session.Values["user_id"] = userId
				session.Save(r, w)
				return nil
			},
			"/",
		),
	)

	r := chi.NewRouter()

	// Middleware
	r.Use(oidcAuthenticationMiddleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public routes
	r.Get("/auth/callback", oidcCallbackHandler)

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
		http.Error(w, "Error fetching feeds", http.StatusInternalServerError)
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
			http.Error(w, "Error fetching posts", http.StatusInternalServerError)
			return
		}
		feedData = append(feedData, FeedData{Feed: f, Posts: posts})
	}

	data := struct {
		Feeds []FeedData
	}{
		Feeds: feedData,
	}

	if err := s.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	userId := s.getUserID(r)

	feeds, err := s.store.GetUserFeeds(userId)
	if err != nil {
		http.Error(w, "Error fetching feeds", http.StatusInternalServerError)
		return
	}

	data := struct {
		Feeds []db.Feed
	}{
		Feeds: feeds,
	}

	if err := s.templates.ExecuteTemplate(w, "settings.html", data); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	content, err := s.fetcher.FetchFeed(r.Context(), url)
	if err != nil {
		http.Error(w, "Invalid feed URL", http.StatusBadRequest)
		return
	}

	feedId, err := s.store.AddFeed(url)
	if err != nil {
		http.Error(w, "Error adding feed", http.StatusInternalServerError)
		return
	}

	for _, item := range content.Items {
		if err := s.store.AddPost(feedId, item.GUID, item.Title, item.Link, item.PublishedAt, item.Content); err != nil {
			log.Printf("Error adding post: %v", err)
		}
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
		http.Error(w, "Error deleting feed", http.StatusInternalServerError)
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
		http.Error(w, "Error marking post as seen", http.StatusInternalServerError)
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
		http.Error(w, "Error marking all posts as seen", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
