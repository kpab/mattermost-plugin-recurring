package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

func getReminders(t *testing.T, tp *testPlugin, userID string) *httptest.ResponseRecorder {
	t.Helper()

	tp.router = tp.initRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reminders", nil)
	if userID != "" {
		req.Header.Set("Mattermost-User-ID", userID)
	}

	w := httptest.NewRecorder()
	tp.ServeHTTP(nil, w, req)

	return w
}

func TestGetRemindersReturnsOwnReminders(t *testing.T) {
	tp := newTestPlugin(t)

	mine := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().Add(time.Hour).UnixMilli())
	require.NoError(t, tp.store.SaveReminder(mine))

	w := getReminders(t, tp, "user1")
	require.Equal(t, http.StatusOK, w.Code)

	var got remindersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Reminders, 1)

	view := got.Reminders[0]
	assert.Equal(t, "r1", view.ID)
	assert.Equal(t, "stand-up", view.Message)
	// Rendered server-side so the sidebar and the slash command always agree.
	assert.Equal(t, "every day at 09:00", view.Schedule)
	assert.NotEmpty(t, view.NextRun)
	assert.False(t, view.Paused)
}

// The listing is scoped to the caller, whose ID comes from the header the
// server sets rather than from anything the client can choose.
func TestGetRemindersDoesNotLeakOtherUsers(t *testing.T) {
	tp := newTestPlugin(t)

	require.NoError(t, tp.store.SaveReminder(
		testReminder("victim", "secret1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())))

	w := getReminders(t, tp, "someone-else")
	require.Equal(t, http.StatusOK, w.Code)

	var got remindersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Empty(t, got.Reminders)
	assert.NotContains(t, w.Body.String(), "secret1")
}

func TestGetRemindersRequiresLogin(t *testing.T) {
	tp := newTestPlugin(t)

	w := getReminders(t, tp, "")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// An empty list has to serialise as [] rather than null, or the sidebar has to
// special-case it before it can map over the result.
func TestGetRemindersEncodesEmptyAsAnArray(t *testing.T) {
	tp := newTestPlugin(t)

	w := getReminders(t, tp, "user1")
	require.Equal(t, http.StatusOK, w.Code)

	assert.Contains(t, w.Body.String(), `"reminders":[]`)
}

func TestGetRemindersShowsPausedState(t *testing.T) {
	tp := newTestPlugin(t)

	paused := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().Add(time.Hour).UnixMilli())
	paused.Paused = true
	require.NoError(t, tp.store.SaveReminder(paused))

	w := getReminders(t, tp, "user1")
	require.Equal(t, http.StatusOK, w.Code)

	var got remindersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Reminders, 1)

	assert.True(t, got.Reminders[0].Paused)
	assert.Equal(t, "paused", got.Reminders[0].NextRun,
		"a paused reminder must not advertise a next run")
}
