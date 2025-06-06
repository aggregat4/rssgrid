package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type OIDCProvider struct {
	config     *oauth2.Config
	provider   *oidc.Provider
	verifier   *oidc.IDTokenVerifier
	stateStore map[string]time.Time
}

func NewOIDCProvider(cfg OIDCConfig) (*OIDCProvider, error) {
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("error creating OIDC provider: %w", err)
	}

	config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	return &OIDCProvider{
		config:     config,
		provider:   provider,
		verifier:   verifier,
		stateStore: make(map[string]time.Time),
	}, nil
}

func (p *OIDCProvider) GenerateAuthURL(w http.ResponseWriter, r *http.Request) (string, error) {
	// Generate random state
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("error generating random state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(b)

	// Store state with timestamp
	p.stateStore[state] = time.Now()

	// Generate auth URL
	authURL := p.config.AuthCodeURL(state)
	return authURL, nil
}

func (p *OIDCProvider) VerifyCallback(r *http.Request) (*oidc.IDToken, error) {
	// Verify state
	state := r.URL.Query().Get("state")
	if state == "" {
		return nil, fmt.Errorf("no state in request")
	}

	// Check if state exists and is not expired (5 minutes)
	if timestamp, exists := p.stateStore[state]; !exists {
		return nil, fmt.Errorf("invalid state")
	} else if time.Since(timestamp) > 5*time.Minute {
		delete(p.stateStore, state)
		return nil, fmt.Errorf("state expired")
	}

	// Clean up state
	delete(p.stateStore, state)

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, fmt.Errorf("no code in request")
	}

	ctx := context.Background()
	token, err := p.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("error exchanging code: %w", err)
	}

	// Extract ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	// Verify ID token
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("error verifying ID token: %w", err)
	}

	return idToken, nil
}
