package poller

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/bungie"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

// Pool manages per-user polling goroutines for event-mode users.
type Pool struct {
	store    store.Store
	api      bungie.API
	getToken func(context.Context, int64, time.Time) (string, error)
	cycler   Cycler

	mu      sync.Mutex
	running map[int64]context.CancelFunc
}

func NewPool(
	s store.Store,
	api bungie.API,
	getToken func(context.Context, int64, time.Time) (string, error),
	c Cycler,
) *Pool {
	return &Pool{
		store:    s,
		api:      api,
		getToken: getToken,
		cycler:   c,
		running:  make(map[int64]context.CancelFunc),
	}
}

// Run scans for event-mode users every scanInterval and manages their goroutines.
// Blocks until ctx is cancelled.
func (p *Pool) Run(ctx context.Context, scanInterval time.Duration) {
	p.scan(ctx)
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			for _, cancel := range p.running {
				cancel()
			}
			p.mu.Unlock()
			return
		case <-ticker.C:
			p.scan(ctx)
		}
	}
}

// scan queries event-mode users and starts/stops goroutines as needed.
func (p *Pool) scan(ctx context.Context) {
	users, err := p.store.EventModeUsers(ctx)
	if err != nil {
		log.Printf("poller: scan failed: %v", err)
		return
	}

	active := make(map[int64]bool, len(users))
	for _, u := range users {
		active[u.ID] = true
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Start goroutines for new event-mode users.
	for _, u := range users {
		if _, ok := p.running[u.ID]; ok {
			continue
		}
		uctx, cancel := context.WithCancel(ctx)
		p.running[u.ID] = cancel
		go func(u store.User) {
			up := &userPoller{
				userID:   u.ID,
				user:     u,
				st:       p.store,
				api:      p.api,
				getToken: p.getToken,
				cycler:   p.cycler,
			}
			up.run(uctx)
			p.mu.Lock()
			delete(p.running, u.ID)
			p.mu.Unlock()
		}(u)
	}

	// Cancel goroutines for users who left event mode.
	for id, cancel := range p.running {
		if !active[id] {
			cancel()
			delete(p.running, id)
		}
	}
}
