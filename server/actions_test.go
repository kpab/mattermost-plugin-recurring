package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// postAction posts an action request as the given user.
func postAction(t *testing.T, tp *testPlugin, path, userID string, context map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(model.PostActionIntegrationRequest{
		UserId:  userID,
		PostId:  "post123",
		Context: context,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions"+path, bytes.NewReader(body))
	req.Header.Set("Mattermost-User-ID", userID)

	w := httptest.NewRecorder()

	tp.router = tp.initRouter()
	tp.ServeHTTP(nil, w, req)

	return w
}

func TestSnoozeAction(t *testing.T) {
	tp := newTestPlugin(t)

	now := time.Now()
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, now.UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	w := postAction(t, tp, "/snooze", "user1", map[string]any{
		contextReminderID:    "r1",
		contextSnoozeMinutes: 15,
	})
	require.Equal(t, http.StatusOK, w.Code)

	stored, err := tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)

	deferred := time.UnixMilli(stored.NextRunAt)
	assert.WithinDuration(t, now.Add(15*time.Minute), deferred, time.Minute,
		"snoozing must move the next run by the requested delay")

	// The schedule itself is untouched, so the reminder returns to its rhythm.
	assert.Equal(t, reminder.KindDaily, stored.Schedule.Kind)
	assert.Equal(t, 9, stored.Schedule.At.Hour)
}

func TestPauseAction(t *testing.T) {
	tp := newTestPlugin(t)

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	w := postAction(t, tp, "/pause", "user1", map[string]any{contextReminderID: "r1"})
	require.Equal(t, http.StatusOK, w.Code)

	stored, err := tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)
	assert.True(t, stored.Paused)
}

// The button's context is client-supplied data. Naming someone else's reminder
// in it must not act on that reminder.
func TestActionsCannotReachAnotherUsersReminder(t *testing.T) {
	tp := newTestPlugin(t)

	victim := testReminder("victim", "secret1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())
	require.NoError(t, tp.store.SaveReminder(victim))

	for _, path := range []string{"/pause", "/snooze"} {
		t.Run(path, func(t *testing.T) {
			w := postAction(t, tp, path, "attacker", map[string]any{
				contextReminderID:    "secret1",
				contextSnoozeMinutes: 15,
			})

			// Answered, but as "gone" — the lookup never saw it.
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "gone")

			stored, err := tp.store.GetReminder("victim", "secret1")
			require.NoError(t, err)
			assert.False(t, stored.Paused, "another user's reminder must be untouched")
			assert.Equal(t, victim.NextRunAt, stored.NextRunAt)
		})
	}
}

func TestActionsRejectAnonymousRequests(t *testing.T) {
	tp := newTestPlugin(t)
	tp.router = tp.initRouter()

	body, err := json.Marshal(model.PostActionIntegrationRequest{Context: map[string]any{contextReminderID: "r1"}})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions/pause", bytes.NewReader(body))
	w := httptest.NewRecorder()

	tp.ServeHTTP(nil, w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestActionsOnADeletedReminder(t *testing.T) {
	tp := newTestPlugin(t)

	w := postAction(t, tp, "/pause", "user1", map[string]any{contextReminderID: "gone"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "gone")
}

func TestActionsRejectAMissingReminderID(t *testing.T) {
	tp := newTestPlugin(t)

	w := postAction(t, tp, "/pause", "user1", map[string]any{})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Buttons post back through the same JSON round trip as any HTTP body, so an
// integer in the context arrives as a float64.
func TestContextInt(t *testing.T) {
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"snooze_minutes":15}`), &decoded))

	got, err := contextInt(decoded, contextSnoozeMinutes)
	require.NoError(t, err)
	assert.Equal(t, 15, got)

	_, err = contextInt(map[string]any{}, contextSnoozeMinutes)
	assert.Error(t, err)

	_, err = contextInt(map[string]any{contextSnoozeMinutes: "fifteen"}, contextSnoozeMinutes)
	assert.Error(t, err)
}

// The buttons have to point at routes that actually exist.
func TestReminderActionsPointAtRegisteredRoutes(t *testing.T) {
	tp := newTestPlugin(t)
	tp.router = tp.initRouter()

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	actions := tp.reminderActions(r)
	require.NotEmpty(t, actions)

	for _, action := range actions {
		require.NotNil(t, action.Integration)

		body, err := json.Marshal(model.PostActionIntegrationRequest{Context: action.Integration.Context})
		require.NoError(t, err)

		// The plugin's own router sees the path with the /plugins/<id> prefix
		// already stripped, which is how Mattermost dispatches to it.
		path := strings.TrimPrefix(action.Integration.URL, "/plugins/"+manifest.Id)

		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Mattermost-User-ID", "user1")
		w := httptest.NewRecorder()

		tp.ServeHTTP(nil, w, req)

		assert.NotEqual(t, http.StatusNotFound, w.Code,
			"button %q posts to %s, which no route handles", action.Name, action.Integration.URL)
	}
}

// Mattermost calls back on /api/v4/posts/<post id>/actions/<action id>, and
// that route matches alphanumerics only. A space 404s — and so does an
// underscore, which URL escaping alone does not catch.
func TestActionIDsAreAlphanumeric(t *testing.T) {
	tp := newTestPlugin(t)

	alphanumeric := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())

	for _, action := range tp.reminderActions(r) {
		t.Run(action.Id, func(t *testing.T) {
			require.NotEmpty(t, action.Id)
			assert.Regexp(t, alphanumeric, action.Id,
				"the callback route matches alphanumerics only, so %q will 404", action.Id)
			assert.Equal(t, action.Id, url.PathEscape(action.Id))
		})
	}
}

// The server hands the integration URL to an HTTP client as-is, so it needs the
// whole plugin path from the site root. A bare "/api/v1/..." fails with
// "unsupported protocol scheme".
func TestActionURLsCarryThePluginPath(t *testing.T) {
	tp := newTestPlugin(t)

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())

	for _, action := range tp.reminderActions(r) {
		require.NotNil(t, action.Integration)
		assert.True(t, strings.HasPrefix(action.Integration.URL, "/plugins/"+manifest.Id+"/"),
			"button %q posts to %q, which does not name the plugin", action.Name, action.Integration.URL)
	}
}

// Every button needs a distinct ID, or the wrong handler answers the press.
func TestActionIDsAreUnique(t *testing.T) {
	tp := newTestPlugin(t)

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())

	seen := map[string]bool{}
	for _, action := range tp.reminderActions(r) {
		require.False(t, seen[action.Id], "duplicate action ID %q", action.Id)
		seen[action.Id] = true
	}
}

// The list's buttons are the only control surface that reaches the mobile app,
// so they have to satisfy the same constraints as the delivered message's.
func TestListActionsAreAlphanumericAndPointAtRoutes(t *testing.T) {
	tp := newTestPlugin(t)
	tp.router = tp.initRouter()

	alphanumeric := regexp.MustCompile(`^[A-Za-z0-9]+$`)

	for _, paused := range []bool{false, true} {
		r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())
		r.Paused = paused
		require.NoError(t, tp.store.SaveReminder(r))

		for _, action := range tp.listActions(r) {
			require.NotNil(t, action.Integration)
			assert.Regexp(t, alphanumeric, action.Id)
			assert.True(t, strings.HasPrefix(action.Integration.URL, "/plugins/"+manifest.Id+"/"))

			body, err := json.Marshal(model.PostActionIntegrationRequest{Context: action.Integration.Context})
			require.NoError(t, err)

			path := strings.TrimPrefix(action.Integration.URL, "/plugins/"+manifest.Id)
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			req.Header.Set("Mattermost-User-ID", "user1")
			w := httptest.NewRecorder()

			tp.ServeHTTP(nil, w, req)

			assert.NotEqual(t, http.StatusNotFound, w.Code,
				"button %q posts to %s, which no route handles", action.Name, action.Integration.URL)
		}
	}
}

// A paused reminder offers Resume, a running one offers Pause.
func TestListActionsToggleWithState(t *testing.T) {
	tp := newTestPlugin(t)

	running := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())
	assert.Equal(t, "Pause", tp.listActions(running)[0].Name)

	paused := running.Clone()
	paused.Paused = true
	assert.Equal(t, "Resume", tp.listActions(paused)[0].Name)
}

func TestResumeAction(t *testing.T) {
	tp := newTestPlugin(t)

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().Add(-time.Hour).UnixMilli())
	r.Paused = true
	require.NoError(t, tp.store.SaveReminder(r))

	w := postAction(t, tp, "/resume", "user1", map[string]any{contextReminderID: "r1"})
	require.Equal(t, http.StatusOK, w.Code)

	stored, err := tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)
	assert.False(t, stored.Paused)
	assert.Greater(t, stored.NextRunAt, time.Now().UnixMilli(),
		"resuming must move the next run into the future, not leave it in the past")
}

func TestDeleteAction(t *testing.T) {
	tp := newTestPlugin(t)

	require.NoError(t, tp.store.SaveReminder(
		testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())))

	w := postAction(t, tp, "/delete", "user1", map[string]any{contextReminderID: "r1"})
	require.Equal(t, http.StatusOK, w.Code)

	_, err := tp.store.GetReminder("user1", "r1")
	assert.ErrorIs(t, err, reminder.ErrNotFound)
}

// Deleting or resuming someone else's reminder must be as impossible as pausing it.
func TestResumeAndDeleteCannotReachAnotherUsersReminder(t *testing.T) {
	tp := newTestPlugin(t)

	victim := testReminder("victim", "secret1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())
	require.NoError(t, tp.store.SaveReminder(victim))

	for _, path := range []string{"/resume", "/delete"} {
		t.Run(path, func(t *testing.T) {
			w := postAction(t, tp, path, "attacker", map[string]any{contextReminderID: "secret1"})
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "gone")

			_, err := tp.store.GetReminder("victim", "secret1")
			assert.NoError(t, err, "another user's reminder must still be there")
		})
	}
}
