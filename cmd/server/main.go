package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/auth"
	"github.com/jsorensen/guardian_shuffle/internal/bungie"
	"github.com/jsorensen/guardian_shuffle/internal/config"
	"github.com/jsorensen/guardian_shuffle/internal/cryptobox"
	"github.com/jsorensen/guardian_shuffle/internal/scheduler"
	"github.com/jsorensen/guardian_shuffle/internal/store"
	"github.com/jsorensen/guardian_shuffle/internal/swap"
	"github.com/jsorensen/guardian_shuffle/internal/web"
)

const bungieBase = "https://www.bungie.net"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pg, err := store.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pg.Close()
	if err := pg.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	box, err := cryptobox.New(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}

	api := bungie.NewClient(cfg.BungieAPIKey, bungieBase, http.DefaultClient)
	tokens := auth.NewTokenManager(pg, box, cfg.BungieClientID, cfg.BungieClientSecret, bungieBase, http.DefaultClient)

	// Emblem hash set: fetched once at boot, refreshed daily in the background.
	var (
		emblemMu  sync.RWMutex
		emblemSet = map[uint32]bool{}
	)
	refreshEmblems := func() bool {
		set, err := api.GetEmblemHashSet(ctx)
		if err != nil {
			log.Printf("manifest: emblem set refresh failed: %v", err)
			return false
		}
		emblemMu.Lock()
		emblemSet = set
		emblemMu.Unlock()
		log.Printf("manifest: loaded %d emblem hashes", len(set))
		return true
	}

	// Initial load with retry.
	func() {
		delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
		if refreshEmblems() {
			return
		}
		for _, d := range delays {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
			if refreshEmblems() {
				return
			}
		}
	}()

	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refreshEmblems()
			}
		}
	}()

	engine := swap.NewEngine(api, pg,
		tokens.ValidAccessToken,
		func() map[uint32]bool {
			emblemMu.RLock()
			defer emblemMu.RUnlock()
			return emblemSet
		},
		func() *rand.Rand { return rand.New(rand.NewSource(time.Now().UnixNano())) },
	)

	sched := scheduler.New(pg, engine)
	go sched.Run(ctx, time.Minute)

	handlers := &web.Handlers{
		Store:        pg,
		Tokens:       tokens,
		Memberships:  api,
		Cycler:       engine,
		Sessions:     web.NewCookieSessions(cfg.HMACKey, cfg.SecureCookies),
		ClientID:     cfg.BungieClientID,
		BaseURL:      cfg.BaseURL,
		AuthorizeURL: bungieBase + "/en/OAuth/Authorize",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handlers.Dashboard)
	mux.HandleFunc("GET /login", handlers.Login)
	mux.HandleFunc("GET /callback", handlers.Callback)
	mux.HandleFunc("POST /settings", handlers.SaveSettings)
	mux.HandleFunc("POST /cycle-now", handlers.CycleNow)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
