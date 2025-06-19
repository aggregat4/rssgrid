package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	baseliboidc "github.com/aggregat4/go-baselib-services/v3/oidc"
	"github.com/aggregat4/rssgrid/internal/config"
	"github.com/aggregat4/rssgrid/internal/db"
	"github.com/aggregat4/rssgrid/internal/feed"
	"github.com/aggregat4/rssgrid/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	store, err := db.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	oidcConfig := baseliboidc.CreateOidcConfiguration(
		cfg.OIDC.IssuerURL,
		cfg.OIDC.ClientID,
		cfg.OIDC.ClientSecret,
		cfg.OIDC.RedirectURL,
	)

	srv, err := server.NewServer(store, oidcConfig, cfg.SessionKey)
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	updater := feed.NewUpdater(store, cfg.UpdateInterval)

	// Create context that will be canceled on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updater.Start(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
		updater.Stop()
		os.Exit(0)
	}()

	log.Printf("Starting server on %s", cfg.Addr)
	if err := srv.Start(cfg.Addr); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
