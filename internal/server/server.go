package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/boris/go-rssgrid/internal/auth"
	"github.com/boris/go-rssgrid/internal/db"
	"github.com/boris/go-rssgrid/internal/feed"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	store     *db.Store
	oidc      *auth.OIDCProvider
	session   *scs.SessionManager
	fetcher   *feed.Fetcher
	templates *template.Template
}

func NewServer(store *db.Store, oidc *auth.OIDCProvider, sessionKey string) (*Server, error) {
	// Initialize session manager
	session := scs.New()
	session.Lifetime = 24 * time.Hour
	session.Cookie.Secure = true
	session.Cookie.HttpOnly = true
	session.Cookie.SameSite = http.SameSiteLaxMode

	// Load templates
	templates, err := template.ParseGlob("internal/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("error loading templates: %w", err)
	}

	return &Server{
		store:     store,
		oidc:      oidc,
		session:   session,
		fetcher:   feed.NewFetcher(),
		templates: templates,
	}, nil
}

func (s *Server) Start(addr string) error {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(s.session.LoadAndSave)

	// Public routes
	r.Get("/login", s.handleLogin)
	r.Get("/auth/callback", s.handleAuthCallback)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)

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

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userId := s.session.GetInt64(r.Context(), "user_id")
		if userId == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	authURL, err := s.oidc.GenerateAuthURL(w, r)
	if err != nil {
		http.Error(w, "Error generating auth URL", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	token, err := s.oidc.VerifyCallback(r)
	if err != nil {
		http.Error(w, "Error verifying callback", http.StatusBadRequest)
		return
	}

	var claims struct {
		Sub string `json:"sub"`
		Iss string `json:"iss"`
	}
	if err := token.Claims(&claims); err != nil {
		http.Error(w, "Error parsing claims", http.StatusInternalServerError)
		return
	}

	userId, err := s.store.GetOrCreateUser(claims.Sub, claims.Iss)
	if err != nil {
		http.Error(w, "Error creating user", http.StatusInternalServerError)
		return
	}

	s.session.Put(r.Context(), "user_id", userId)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userId := s.session.GetInt64(r.Context(), "user_id")
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
	userId := s.session.GetInt64(r.Context(), "user_id")
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

	// Fetch and validate feed
	content, err := s.fetcher.FetchFeed(r.Context(), url)
	if err != nil {
		http.Error(w, "Invalid feed URL", http.StatusBadRequest)
		return
	}

	// Add feed to database
	feedId, err := s.store.AddFeed(url)
	if err != nil {
		http.Error(w, "Error adding feed", http.StatusInternalServerError)
		return
	}

	// Add posts
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

	userId := s.session.GetInt64(r.Context(), "user_id")
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

	userId := s.session.GetInt64(r.Context(), "user_id")
	if err := s.store.MarkAllFeedPostsAsSeen(userId, feedId); err != nil {
		http.Error(w, "Error marking all posts as seen", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
