// Package scheduler runs periodic guardian cycling for due users.
package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/store"
)

// Cycler is the swap.Engine surface the scheduler needs.
type Cycler interface {
	CycleUser(ctx context.Context, userID int64, now time.Time) error
}

type Scheduler struct {
	store  store.Store
	cycler Cycler
}

func New(s store.Store, c Cycler) *Scheduler {
	return &Scheduler{store: s, cycler: c}
}

// RunOnce cycles every user that is currently due.
func (s *Scheduler) RunOnce(ctx context.Context, now time.Time) error {
	due, err := s.store.DueUsers(ctx, now)
	if err != nil {
		return err
	}
	for _, id := range due {
		if err := s.cycler.CycleUser(ctx, id, now); err != nil {
			log.Printf("scheduler: cycle user %d failed: %v", id, err)
		}
	}
	return nil
}

// Run ticks every interval until the context is cancelled.
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			if err := s.RunOnce(ctx, t); err != nil {
				log.Printf("scheduler: run failed: %v", err)
			}
		}
	}
}
