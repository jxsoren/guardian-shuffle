package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)


type Memory struct {
	mu       sync.Mutex
	nextID   int64
	byMember map[string]int64
	users    map[int64]User
	tokens   map[int64]Tokens
	settings map[int64]Settings
}

func NewMemory() *Memory {
	return &Memory{
		nextID:   1,
		byMember: map[string]int64{},
		users:    map[int64]User{},
		tokens:   map[int64]Tokens{},
		settings: map[int64]Settings{},
	}
}

func (m *Memory) UpsertUser(_ context.Context, u User) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byMember[u.BungieMembershipID]; ok {
		u.ID = id
		m.users[id] = u
		return id, nil
	}
	id := m.nextID
	m.nextID++
	u.ID = id
	m.byMember[u.BungieMembershipID] = id
	m.users[id] = u
	return id, nil
}

func (m *Memory) GetUser(_ context.Context, id int64) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return User{}, fmt.Errorf("user %d not found", id)
	}
	return u, nil
}

func (m *Memory) SaveTokens(_ context.Context, t Tokens) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[t.UserID] = t
	return nil
}

func (m *Memory) GetTokens(_ context.Context, userID int64) (Tokens, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[userID]
	if !ok {
		return Tokens{}, fmt.Errorf("tokens for user %d not found", userID)
	}
	return t, nil
}

func (m *Memory) SaveSettings(_ context.Context, s Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[s.UserID] = s
	return nil
}

func (m *Memory) GetSettings(_ context.Context, userID int64) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.settings[userID]
	if !ok {
		return Settings{UserID: userID, TriggerMode: "manual"}, nil
	}
	return s, nil
}

func (m *Memory) DueUsers(_ context.Context, now time.Time) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var due []int64
	for id, s := range m.settings {
		if !s.Enabled || s.TriggerMode != "scheduled" || s.IntervalSeconds <= 0 {
			continue
		}
		if s.LastCycledAt == nil || !now.Before(s.LastCycledAt.Add(time.Duration(s.IntervalSeconds)*time.Second)) {
			due = append(due, id)
		}
	}
	return due, nil
}

func (m *Memory) RecordSwap(_ context.Context, userID int64, fromHash, toHash uint32, status string) error {
	return nil // history retention is Phase 1-optional; no-op for the in-memory store
}
