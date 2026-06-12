// Package store defines the Store interface and domain structs used across the guardian_shuffle service.
package store

import (
	"context"
	"time"
)

type User struct {
	ID                 int64
	BungieMembershipID string
	MembershipType     int64
	PrimaryCharacterID string
}

type Tokens struct {
	UserID           int64
	AccessTokenEnc   []byte
	RefreshTokenEnc  []byte
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type Settings struct {
	UserID          int64
	Enabled         bool
	TriggerMode     string // "manual" | "scheduled"
	IntervalSeconds int64
	LastCycledAt    *time.Time
}

type Store interface {
	// UpsertUser inserts or updates by BungieMembershipID and returns the user ID.
	UpsertUser(ctx context.Context, u User) (int64, error)
	GetUser(ctx context.Context, id int64) (User, error)
	SaveTokens(ctx context.Context, t Tokens) error
	GetTokens(ctx context.Context, userID int64) (Tokens, error)
	GetSettings(ctx context.Context, userID int64) (Settings, error)
	SaveSettings(ctx context.Context, s Settings) error
	// DueUsers returns IDs of enabled, scheduled users whose interval has elapsed by now.
	DueUsers(ctx context.Context, now time.Time) ([]int64, error)
	RecordSwap(ctx context.Context, userID int64, fromHash, toHash uint32, status string) error
}
