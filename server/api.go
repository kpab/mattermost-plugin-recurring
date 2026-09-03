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

	// The API surface is added in M2, when the right-hand sidebar needs it.
	// The prefix and the auth middleware are set up now so that every route
	// added later is behind the login check by construction. Note that mux only
	// runs middleware for requests that match a route, so with no routes
	// registered the middleware never runs and everything is a 404.
	router.PathPrefix("/api/v1").Subrouter()

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
