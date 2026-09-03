package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// initRouter initializes the HTTP router for the plugin.
func (p *Plugin) initRouter() *mux.Router {
	router := mux.NewRouter()

	// Middleware to require that the user is logged in
	router.Use(p.MattermostAuthorizationRequired)

	api := router.PathPrefix("/api/v1").Subrouter()

	// Buttons on a delivered reminder post back here. Mattermost forwards the
	// pressing user in Mattermost-User-ID, which the middleware above requires,
	// and each handler scopes its lookup to that user's own reminders.
	actions := api.PathPrefix("/actions").Subrouter()
	actions.HandleFunc("/snooze", p.handleSnooze).Methods(http.MethodPost)
	actions.HandleFunc("/pause", p.handlePause).Methods(http.MethodPost)

	return router
}

// ServeHTTP routes requests sent to
// <siteUrl>/plugins/com.github.kpab.recurring/.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
