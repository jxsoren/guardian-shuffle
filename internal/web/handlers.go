// Package web implements the HTTP handlers for Guardian Shuffle's web UI.
package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jsorensen/guardian_shuffle/internal/auth"
	"github.com/jsorensen/guardian_shuffle/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

var tmpl = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

// Cycler is the swap.Engine surface the handlers need.
type Cycler interface {
	CycleUser(ctx context.Context, userID int64, now time.Time) error
}

// SessionManager maps requests to a logged-in user ID. Phase 1 uses a signed cookie.
type SessionManager interface {
	UserID(r *http.Request) (int64, bool)
	SetUserID(w http.ResponseWriter, userID int64)
}

type Handlers struct {
	Store        store.Store
	Tokens       *auth.TokenManager
	Cycler       Cycler
	Sessions     SessionManager
	ClientID     string
	BaseURL      string
	AuthorizeURL string // https://www.bungie.net/en/OAuth/Authorize
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {h.ClientID},
	}
	http.Redirect(w, r, h.AuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

// Callback handles the OAuth redirect: exchange code, look up membership, create session.
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	resp, err := h.Tokens.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}
	id, err := h.Store.UpsertUser(r.Context(), store.User{BungieMembershipID: resp.MembershipID})
	if err != nil {
		http.Error(w, "user upsert failed", http.StatusInternalServerError)
		return
	}
	if err := h.Tokens.Persist(r.Context(), id, resp, time.Now()); err != nil {
		http.Error(w, "token persist failed", http.StatusInternalServerError)
		return
	}
	h.Sessions.SetUserID(w, id)
	http.Redirect(w, r, "/", http.StatusFound)
}

type dashboardData struct {
	LoggedIn        bool
	MembershipID    string
	Enabled         bool
	Mode            string
	IntervalSeconds int64
}

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	data := dashboardData{Mode: "manual"}
	if id, ok := h.Sessions.UserID(r); ok {
		data.LoggedIn = true
		if u, err := h.Store.GetUser(r.Context(), id); err == nil {
			data.MembershipID = u.BungieMembershipID
		}
		if s, err := h.Store.GetSettings(r.Context(), id); err == nil {
			data.Enabled, data.Mode, data.IntervalSeconds = s.Enabled, s.TriggerMode, s.IntervalSeconds
		}
	}
	if err := tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handlers) SaveSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := h.Sessions.UserID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = r.ParseForm()
	interval, _ := strconv.ParseInt(r.FormValue("interval_seconds"), 10, 64)
	mode := r.FormValue("mode")
	if mode != "scheduled" {
		mode = "manual"
	}
	cur, _ := h.Store.GetSettings(r.Context(), id)
	cur.UserID = id
	cur.Enabled = r.FormValue("enabled") != ""
	cur.TriggerMode = mode
	cur.IntervalSeconds = interval
	if err := h.Store.SaveSettings(r.Context(), cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handlers) CycleNow(w http.ResponseWriter, r *http.Request) {
	id, ok := h.Sessions.UserID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.Cycler.CycleUser(r.Context(), id, time.Now()); err != nil {
		fmt.Fprintf(w, "Cycle failed: %v", err)
		return
	}
	fmt.Fprint(w, "Emblem cycled!")
}
