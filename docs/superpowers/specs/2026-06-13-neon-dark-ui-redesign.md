# Guardian Shuffle — Neon Dark UI Redesign

**Date:** 2026-06-13
**Status:** Approved design, ready for planning

## Problem

The web UI is a single unstyled `internal/web/templates/dashboard.html` — bare HTML
with htmx and no CSS. It works but looks lackluster. Make it sleek and modern.

## Direction

**Neon Dark:** a dark canvas with violet→cyan gradient accents and subtle background
glow — energetic, modern-gaming. Chosen over an on-theme "cosmic" gold/amber look and a
light minimal SaaS look.

## Scope

Restyle the existing two states **and** modernize the controls. Same features, no new
backend data:

- Checkbox → toggle switch
- Mode `<select>` → segmented control (Scheduled / Manual / After game)
- Interval field reveals only when Scheduled is active

**Out of scope** (would need new backend data): emblem image preview, "last cycled"
timestamp, live activity-state display.

## Visual System

| Token | Value |
|---|---|
| Canvas bg | `#0c0d14` |
| Text | `#e6e7ee` |
| Muted text | `#8b8ea3` |
| Accent gradient | `#a78bfa → #22d3ee` (violet → cyan) |
| Card surface | `rgba(255,255,255,.03)` |
| Card border | `rgba(124,77,255,.28)` |
| Success pill | cyan/green: bg `rgba(34,211,238,.10)`, border `rgba(34,211,238,.35)`, text `#7fe9ff` |
| Error pill | amber/red: warm bg/border/text for the "in an activity" and generic-error cases |

- Background: two soft radial glows (violet top-left, cyan bottom-right).
- Cards: translucent surface, violet-tinted border, rounded corners (~14px).
- Fonts via Google Fonts CDN — **Space Grotesk** (headings/wordmark), **Inter** (body).
  Consistent with the existing CDN approach (htmx already loads from unpkg).

## Layout & Components

### Signed out
Centered hero: rounded logo tile (gradient, glowing), gradient wordmark "Guardian
Shuffle", one-line tagline, prominent gradient **Sign in with Bungie** button linking to
`/login`.

### Signed in
1. Header: small logo tile + gradient wordmark + `membership <id>…` subtitle.
2. Settings card (`<form method="post" action="/settings">`):
   - **Auto-shuffle toggle** — styled native checkbox `name="enabled"`.
   - **Mode segmented control** — three styled native radios `name="mode"`
     (`scheduled` / `manual` / `event`), preserving the current values.
   - **Interval field** — `name="interval_seconds"`, number input, `min=60`. Shown only
     when Scheduled is selected (small vanilla JS toggle on the radio group).
   - **Save settings** submit button (secondary style).
3. **Cycle now** — large primary gradient button (`hx-post="/cycle-now"`,
   `hx-target="#result"`, `hx-swap="innerHTML"`).
4. `#result` — target for the cycle-now status pill.

## Technical Approach

- **CSS location:** a single `<style>` block inside `dashboard.html`. The page is one
  embedded template; no static-file route is added to the `net/http` ServeMux.
- **Form semantics unchanged:** controls are styled native inputs, so the form still
  POSTs to `/settings` and the redirect-back flow is identical. No changes to
  `SaveSettings`, the store, or any other backend.
- **Interval reveal:** minimal inline vanilla JS that shows/hides the interval field
  based on the selected mode radio. Progressive enhancement — if JS is off, the field
  is simply always visible.
- **Result pill styling:** `CycleNow` currently writes a bare text string via
  `fmt.Fprint`. Change it to emit a small HTML fragment wrapping the message in an
  element with a `success` or `error` class (e.g. `<div class="pill pill-ok">…</div>`).
  Still HTTP 200, still swapped into `#result` by htmx — observable behavior unchanged,
  only now styleable. Three messages keep their current wording:
  - success: "Emblem cycled!" → success pill
  - in-activity (`bungie.ErrItemActionForbidden`): existing wording → error pill
  - generic error: existing wording → error pill

## Testing

- `go build ./...` and `go test ./...` pass.
- `internal/web/handlers_test.go` — existing `CycleNow` tests use `strings.Contains`
  ("cycled", "activity") and a no-leak check, not exact-body matches. Preserving the
  message wording inside the new fragment keeps them green; no test changes expected.
- Manual: `go run ./cmd/server`, load `/` signed out (hero), sign in, verify toggle,
  segmented mode, interval reveal on Scheduled, Save settings round-trips, and Cycle now
  renders the styled pill for success and the in-activity/error cases.

## Files Touched

- `internal/web/templates/dashboard.html` — full restyle + modernized controls + inline
  `<style>` and interval-reveal script.
- `internal/web/handlers.go` — `CycleNow` emits styled HTML fragments.
- `internal/web/handlers_test.go` — no changes expected (assertions use `strings.Contains`).
