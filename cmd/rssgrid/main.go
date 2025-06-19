package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	baseliboidc "github.com/aggregat4/go-baselib-services/v3/oidc"
	"github.com/boris/go-rssgrid/internal/config"
	"github.com/boris/go-rssgrid/internal/db"
	"github.com/boris/go-rssgrid/internal/feed"
	"github.com/boris/go-rssgrid/internal/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Initialize database
	store, err := db.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	// Create OIDC configuration
	oidcConfig := baseliboidc.CreateOidcConfiguration(
		cfg.OIDC.IssuerURL,
		cfg.OIDC.ClientID,
		cfg.OIDC.ClientSecret,
		cfg.OIDC.RedirectURL,
	)

	// Initialize server
	srv, err := server.NewServer(store, oidcConfig, cfg.SessionKey)
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	// Initialize feed updater
	updater := feed.NewUpdater(store, cfg.UpdateInterval)

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
	log.Printf("Starting server on %s", cfg.Addr)
	if err := srv.Start(cfg.Addr); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
