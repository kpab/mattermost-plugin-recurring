package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server rejects KV keys longer than 50 characters, and the scheduler
// prefixes our key with "once_" for the job record and "mutex_" for the lock it
// takes while running. If a job key ever outgrows that budget, reminders fail
// to schedule at runtime rather than at compile time — so pin it down here.
func TestJobKeyFitsInTheKVKeyLimit(t *testing.T) {
	const (
		kvKeyLimit         = 50
		longestKeyPrefix   = "mutex_"
		mattermostIDLength = 26
	)

	key := jobKey(model.NewId(), newReminderID())

	require.Len(t, model.NewId(), mattermostIDLength, "assumption about user ID length no longer holds")
	assert.LessOrEqual(t, len(longestKeyPrefix)+len(key), kvKeyLimit,
		"job key %q leaves no room for the scheduler's own prefix", key)
}

func TestParseJobKey(t *testing.T) {
	userID := model.NewId()
	reminderID := newReminderID()

	t.Run("round trips", func(t *testing.T) {
		gotUser, gotReminder, ok := parseJobKey(jobKey(userID, reminderID))

		require.True(t, ok)
		assert.Equal(t, userID, gotUser)
		assert.Equal(t, reminderID, gotReminder)
	})

	for name, key := range map[string]string{
		"empty":            "",
		"no separator":     "justonevalue",
		"missing user":     "_" + reminderID,
		"missing reminder": userID + "_",
		"separator only":   "_",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, _, ok := parseJobKey(key)
			assert.False(t, ok)
		})
	}
}

func TestNewReminderIDIsUnique(t *testing.T) {
	seen := make(map[string]bool, 1000)

	for range 1000 {
		id := newReminderID()
		require.Len(t, id, reminderIDLength)
		require.False(t, seen[id], "generated a duplicate reminder ID: %s", id)
		seen[id] = true
	}
}
