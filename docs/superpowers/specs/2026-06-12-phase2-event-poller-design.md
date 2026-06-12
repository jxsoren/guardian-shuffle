# Guardian Shuffle — Phase 2: Event-Based Poller Design

**Date:** 2026-06-12
**Status:** Approved

## Summary

Add an "After each game" trigger mode. A per-user goroutine polls the player's active character for activity state, detects the in-activity → orbit transition, and fires the swap engine when the game ends.

## Goals

- Let users set `TriggerMode = "event"` and have their emblem cycled automatically after each completed game.
- Poll only users who need it; back off when idle to respect Bungie rate limits.
- Self-healing: a server restart re-bootstraps from the DB without missing transitions.

## Architecture

One new package `internal/poller` with a single exported type `Pool`. Two concurrent roles:

**Coordinator loop** — runs every 30 s. Queries `store.EventModeUsers()` for all enabled event-mode users. Starts a goroutine for each user that doesn't have one; cancels goroutines for users who left event mode. State: `map[userID]context.CancelFunc` under a mutex.

**Per-user loop** — runs a variable-rate timer. Calls `bungie.GetCharacterActivities` on each tick. Drives a three-state machine; fires a cycle on `InActivity → InOrbit`. Loads initial state from the DB on startup.

```
Unknown ──► InActivity ──► InOrbit ──► cycle fired
              ▲                │
              └────────────────┘  (InOrbit → InActivity: switch to fast timer, no cycle)
```

Poll intervals: **30 s** when `InActivity`, **120 s** when `InOrbit` or `Unknown`.

`Pool` depends on `store.Store`, `bungie.API`, a `ValidAccessToken` func, and a `Cycler` interface — the same surface already used by `swap.Engine` and `scheduler.Scheduler`.

## Components

### `internal/poller` (new package)

| File | Responsibility |
|---|---|
| `pool.go` | `Pool` type, coordinator loop, goroutine lifecycle |
| `user_poller.go` | Per-user poll loop, state machine, cycle trigger |
| `pool_test.go` | Coordinator scan, goroutine start/stop |
| `user_poller_test.go` | State machine transitions, interval switching, reauth exit |

### `internal/bungie` (extend)

New method on `*Client` and `API` interface:

```go
GetCharacterActivities(ctx context.Context, token string, mType int64, mID, charID string) (uint32, error)
```

Calls `GET /Platform/Destiny2/{mType}/Profile/{mID}/Character/{charID}/?components=204`.

New response type in `types.go`:

```go
type CharacterActivitiesResponse struct {
    Response struct {
        Activities struct {
            Data struct {
                CurrentActivityHash uint32 `json:"currentActivityHash"`
            } `json:"data"`
        } `json:"activities"` // per-character endpoint uses "activities", not "characterActivities"
    } `json:"Response"`
    ErrorCode   int    `json:"ErrorCode"`
    ErrorStatus string `json:"ErrorStatus"`
    Message     string `json:"Message"`
}
```

### `internal/store` (extend)

New struct:

```go
type ActivityState struct {
    UserID       int64
    CharID       string
    ActivityHash uint32
    UpdatedAt    time.Time
}
```

New `Store` methods:

```go
// EventModeUsers returns all enabled users with TriggerMode == "event".
EventModeUsers(ctx context.Context) ([]User, error)

// GetActivityState returns the persisted state; returns zero-value (not error) if none exists.
GetActivityState(ctx context.Context, userID int64) (ActivityState, error)

// SaveActivityState upserts the activity state for a user.
SaveActivityState(ctx context.Context, s ActivityState) error
```

`TriggerMode` comment updated: `"manual" | "scheduled" | "event"`.

New migration `internal/store/migrations/0002_activity_state.sql`:

```sql
CREATE TABLE activity_states (
    user_id       BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    char_id       TEXT        NOT NULL DEFAULT '',
    activity_hash BIGINT      NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `internal/web` (extend)

**`handlers.go`** — `SaveSettings`: accept `"event"` as a valid mode:

```go
switch mode {
case "scheduled", "event":
    // keep
default:
    mode = "manual"
}
```

**`templates/dashboard.html`** — add option to mode selector:

```html
<option value="event" {{if eq .Mode "event"}}selected{{end}}>After each game</option>
```

### `cmd/server/main.go` (extend)

After the scheduler startup:

```go
pool := poller.NewPool(pg, api, tokens.ValidAccessToken, engine)
go pool.Run(ctx, 30*time.Second)
```

## Data Flow

1. User sets mode to "After each game" in dashboard → `TriggerMode = "event"` saved to DB.
2. Coordinator scan (next tick, ≤30 s) calls `EventModeUsers`, sees the user, starts a goroutine.
3. Per-user goroutine loads `ActivityState` from DB; derives initial state (`hash == 0` → `InOrbit`, else → `InActivity`; missing → `Unknown`).
4. Each tick: call `GetCharacterActivities`. Transition state machine. Save new `ActivityState` to DB.
5. On `InActivity → InOrbit`: call `cycler.CycleUser`. Reset timer to slow interval.
6. On `InOrbit → InActivity`: reset timer to fast interval.
7. User disables or changes mode: coordinator cancels the goroutine on next scan.

## Error Handling

| Condition | Response |
|---|---|
| `ErrReauthRequired` | Goroutine exits; coordinator logs and skips restart until fresh tokens exist |
| Bungie rate limit | Log; sleep the returned `ThrottleSeconds`; resume |
| Transient network error | Log; skip tick; retry at next interval |
| `PrimaryCharacterID` empty | Log; use slow interval; retry each tick |
| `SystemDisabled` error code | Log; use slow interval until cleared |
| Equip fails (character in activity) | `swap.Engine` already handles this; poller does nothing extra |

## Testing Strategy

**`internal/poller`** (unit, fake API):
- State machine: inject hash sequences `[nonzero, 0]` → verify cycle fires once; `[0, nonzero, 0]` → verify cycle fires; `[0, 0]` → no cycle; `[nonzero, nonzero]` → no cycle
- `Unknown` initial state: `[0]` → no cycle; `[nonzero, 0]` → cycle fires (second transition, not first)
- Reauth: verify goroutine exits when `ValidAccessToken` returns `ErrReauthRequired`
- Interval: verify 30 s timer used when `InActivity`, 120 s when `InOrbit`/`Unknown`
- Coordinator: `scan()` starts goroutines for event-mode users; stops them when users leave event mode

**`internal/store`** (unit + Postgres integration):
- `EventModeUsers` returns only enabled event-mode users
- `GetActivityState` returns zero-value for missing user (no error)
- `SaveActivityState` / `GetActivityState` round-trip

**`internal/bungie`** (unit, mock HTTP server):
- `GetCharacterActivities` parses `currentActivityHash` correctly
- Non-1 `ErrorCode` returns an error

**`internal/web`** (unit):
- `SaveSettings` stores `"event"` mode correctly
- `SaveSettings` coerces unknown mode to `"manual"`

## Non-Goals

- Polling `GetActivityHistory` (we use `currentActivityHash` only)
- Multi-character support
- Adaptive rate limiting based on global app quota
- Alerting the user when the poller stops due to reauth
