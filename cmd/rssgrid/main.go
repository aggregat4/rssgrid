package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boris/go-rssgrid/internal/auth"
	"github.com/boris/go-rssgrid/internal/db"
	"github.com/boris/go-rssgrid/internal/feed"
	"github.com/boris/go-rssgrid/internal/server"
)

func main() {
	// Parse command line flags
	addr := flag.String("addr", ":8080", "HTTP server address")
	dbPath := flag.String("db", "rssgrid.db", "Path to SQLite database file")
	updateInterval := flag.Duration("update-interval", 30*time.Minute, "Feed update interval")
	flag.Parse()

	// Get required environment variables
	oidcIssuer := os.Getenv("MONOCLE_OIDC_ISSUER_URL")
	oidcClientID := os.Getenv("MONOCLE_OIDC_CLIENT_ID")
	oidcClientSecret := os.Getenv("MONOCLE_OIDC_CLIENT_SECRET")
	sessionKey := os.Getenv("MONOCLE_SESSION_KEY")

	if oidcIssuer == "" || oidcClientID == "" || oidcClientSecret == "" || sessionKey == "" {
		log.Fatal("Missing required environment variables. Please set MONOCLE_OIDC_ISSUER_URL, MONOCLE_OIDC_CLIENT_ID, MONOCLE_OIDC_CLIENT_SECRET, and MONOCLE_SESSION_KEY")
	}

	// Initialize database
	store, err := db.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	// Initialize OIDC provider
	oidcConfig := auth.OIDCConfig{
		IssuerURL:    oidcIssuer,
		ClientID:     oidcClientID,
		ClientSecret: oidcClientSecret,
		RedirectURL:  "http://localhost:8080/auth/callback", // TODO: Make configurable
	}

	oidcProvider, err := auth.NewOIDCProvider(oidcConfig)
	if err != nil {
		log.Fatalf("Error initializing OIDC provider: %v", err)
	}

	// Initialize server
	srv, err := server.NewServer(store, oidcProvider, sessionKey)
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	// Initialize feed updater
	updater := feed.NewUpdater(store, *updateInterval)

	// Create context that will be canceled on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start feed updater
	updater.Start(ctx)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
		updater.Stop()
		os.Exit(0)
	}()

	// Start server
	log.Printf("Starting server on %s", *addr)
	if err := srv.Start(*addr); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
