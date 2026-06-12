# Guardian Shuffle

Cycles your Destiny 2 active character's equipped emblem — manually or on a fixed interval.

## How it works

You sign in with your Bungie account (OAuth2). Guardian Shuffle reads the emblems in
your most-recently-played character's inventory and equips a random one — either when
you click **Cycle now** or automatically on an interval you choose. Emblems are
identified from the Destiny manifest; equips happen through the Bungie API (the same
mechanism third-party tools like DIM use).

> Note: the Bungie API refuses to equip an emblem while your character is *in an
> activity*. Cycling works in orbit, social spaces, or while offline. A scheduled
> cycle that lands mid-activity is skipped and retried on the next tick.

## Setup

1. Register an app at https://www.bungie.net/en/Application
   - OAuth Client Type: **Confidential**
   - Redirect URL: `${BASE_URL}/callback`
   - Scope: **Move or Equip Destiny Items**
2. Create a Postgres database.
3. Set environment variables:

   | Var | Example |
   |---|---|
   | `DATABASE_URL` | `postgres://localhost:5432/guardian?sslmode=disable` |
   | `BUNGIE_API_KEY` | from the Bungie app page |
   | `BUNGIE_CLIENT_ID` | from the Bungie app page |
   | `BUNGIE_CLIENT_SECRET` | from the Bungie app page |
   | `BASE_URL` | `http://localhost:8080` |
   | `TOKEN_ENCRYPTION_KEY` | a 32-byte random string |
   | `LISTEN_ADDR` | `:8080` (optional) |

## Run

```bash
go run ./cmd/server
```

Open http://localhost:8080, sign in with Bungie, then use **Cycle now** or enable
**Scheduled** mode with an interval.

## Test

```bash
go test ./...
# include the Postgres integration test (requires a running Postgres):
createdb guardian_test
TEST_DATABASE_URL=postgres://localhost:5432/guardian_test?sslmode=disable go test ./internal/store/...
```

## Manual smoke test

With the env configured and Postgres running:

1. `go run ./cmd/server`.
2. Visit `/`, click **Sign in with Bungie**, approve.
3. Back at `/`, click **Cycle now** while your character is **in orbit**. Confirm in-game
   (or via a second profile fetch) that the equipped emblem changed.
4. Set **Scheduled**, interval 60s, **Enabled**, save. Confirm the emblem rotates ~once
   per minute while in orbit.
5. While **in an activity**, confirm **Cycle now** reports a failure message rather than
   crashing.

## Architecture (Phase 1)

A single Go binary running three concurrent roles that share one swap engine:

- **Web/API** — OAuth login/callback, dashboard, settings, "Cycle now" (`internal/web`).
- **Scheduler** — interval ticker that cycles due users (`internal/scheduler`).
- **Swap engine** — picks a random inventory emblem for the active character and equips
  it (`internal/swap`).

Supporting packages: `config` (env), `cryptobox` (AES-256-GCM token encryption),
`bungie` (API client + manifest), `auth` (OAuth token manager with refresh), `store`
(Postgres + in-memory implementations of a shared `Store` interface).

## Known limitations / Phase 1 follow-ups

These are intentional Phase 1 scope cuts and hardening items to address before any
internet-facing, multi-user deployment:

- **OAuth `state` / login-CSRF (security — do before production):** the login flow does
  not yet generate or validate an OAuth `state` parameter. Add a random state in a
  pre-auth cookie and verify it in the callback.
- **Separate signing key (security):** the 32-byte `TOKEN_ENCRYPTION_KEY` is currently
  used both for AES-GCM token encryption and HMAC session-cookie signing. Derive two
  subkeys (e.g. HKDF) or add a second secret.
- **`Secure` cookie flag:** the session cookie is `HttpOnly` + `SameSite=Lax` but not
  `Secure`. Set `Secure` when serving over HTTPS.
- **Generic error responses:** some handlers surface internal error strings; replace with
  generic messages before public exposure.
- **Manifest fetch resilience:** if the initial emblem-manifest fetch fails at boot, the
  shuffle pool is empty (cycling no-ops) until the next 24h refresh. Add retry/backoff.
- **Postgres integration test is environment-gated:** it skips unless `TEST_DATABASE_URL`
  is set, so CI must provision a Postgres to exercise the SQL layer.

## Roadmap

- **Phase 2:** event-based trigger — cycle after every completed game (poll the player's
  activity state and fire on the in-activity → orbit transition).
- **Phase 3:** cron-style schedules.
- **Deferred:** Collections-wide emblem pool, multiple characters, ordered playlists,
  curated favorite pools.
