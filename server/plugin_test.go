package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The plugin serves no routes yet — the API surface arrives with the right-hand
// sidebar in M2 — so anything asked of it is a 404.
func TestServeHTTPHasNoRoutesYet(t *testing.T) {
	plugin := Plugin{}
	plugin.router = plugin.initRouter()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	r.Header.Set("Mattermost-User-ID", "test-user-id")

	plugin.ServeHTTP(nil, w, r)

	result := w.Result()
	defer func() { _ = result.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, result.StatusCode)
}

// Every route added later is wrapped in this, so it is tested directly rather
// than through the router: mux only runs middleware for requests that match a
// route, and there are no routes to match yet.
func TestMattermostAuthorizationRequired(t *testing.T) {
	plugin := Plugin{}

	reached := false
	handler := plugin.MattermostAuthorizationRequired(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("anonymous requests are rejected", func(t *testing.T) {
		reached = false
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil))

		result := w.Result()
		defer func() { _ = result.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, result.StatusCode)
		assert.False(t, reached, "the handler must not run for an anonymous request")
	})

	t.Run("logged-in requests pass through", func(t *testing.T) {
		reached = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
		r.Header.Set("Mattermost-User-ID", "test-user-id")

		handler.ServeHTTP(w, r)

		result := w.Result()
		defer func() { _ = result.Body.Close() }()

		assert.Equal(t, http.StatusOK, result.StatusCode)
		assert.True(t, reached)
	})
}
