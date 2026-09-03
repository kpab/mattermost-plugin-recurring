package kvstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

const testUserID = "user1234567890123456789012"

// newReminder builds a valid weekly reminder for tests to vary.
func newReminder(id string, createdAt int64) *reminder.Reminder {
	return &reminder.Reminder{
		ID:      id,
		UserID:  testUserID,
		Message: "weekly report",
		Schedule: reminder.Schedule{
			Kind:     reminder.KindWeekly,
			At:       reminder.TimeOfDay{Hour: 10, Minute: 0},
			Weekdays: []time.Weekday{time.Monday},
		},
		Timezone:  "Asia/Tokyo",
		NextRunAt: 1_700_000_000_000,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

func TestGetRemindersWhenEmpty(t *testing.T) {
	store, _ := newTestStore(t)

	reminders, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	assert.Empty(t, reminders)
}

func TestSaveAndGetReminder(t *testing.T) {
	store, _ := newTestStore(t)

	want := newReminder("abc", 100)
	require.NoError(t, store.SaveReminder(want))

	got, err := store.GetReminder(testUserID, "abc")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	all, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, want, all[0])
}

func TestSaveReminderReplacesSameID(t *testing.T) {
	store, _ := newTestStore(t)

	require.NoError(t, store.SaveReminder(newReminder("abc", 100)))

	updated := newReminder("abc", 100)
	updated.Message = "updated message"
	require.NoError(t, store.SaveReminder(updated))

	all, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	require.Len(t, all, 1, "saving the same ID must replace, not append")
	assert.Equal(t, "updated message", all[0].Message)
}

func TestGetReminderNotFound(t *testing.T) {
	store, _ := newTestStore(t)

	require.NoError(t, store.SaveReminder(newReminder("abc", 100)))

	_, err := store.GetReminder(testUserID, "nope")
	assert.ErrorIs(t, err, reminder.ErrNotFound)
}

func TestRemindersAreSortedByCreatedAt(t *testing.T) {
	store, _ := newTestStore(t)

	require.NoError(t, store.SaveReminder(newReminder("third", 300)))
	require.NoError(t, store.SaveReminder(newReminder("first", 100)))
	require.NoError(t, store.SaveReminder(newReminder("second", 200)))

	all, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, []string{"first", "second", "third"}, []string{all[0].ID, all[1].ID, all[2].ID})
}

func TestDeleteReminder(t *testing.T) {
	store, _ := newTestStore(t)

	require.NoError(t, store.SaveReminder(newReminder("keep", 100)))
	require.NoError(t, store.SaveReminder(newReminder("drop", 200)))

	require.NoError(t, store.DeleteReminder(testUserID, "drop"))

	all, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "keep", all[0].ID)
}

func TestDeleteMissingReminderIsNotAnError(t *testing.T) {
	store, _ := newTestStore(t)

	require.NoError(t, store.SaveReminder(newReminder("abc", 100)))

	assert.NoError(t, store.DeleteReminder(testUserID, "nope"))

	all, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestDeletingLastReminderRemovesTheKey(t *testing.T) {
	store, api := newTestStore(t)

	require.NoError(t, store.SaveReminder(newReminder("abc", 100)))
	require.True(t, api.has(remindersKey(testUserID)))

	require.NoError(t, store.DeleteReminder(testUserID, "abc"))

	assert.False(t, api.has(remindersKey(testUserID)),
		"an empty list should leave no row behind")
}

func TestDeleteAllReminders(t *testing.T) {
	store, api := newTestStore(t)

	require.NoError(t, store.SaveReminder(newReminder("a", 100)))
	require.NoError(t, store.SaveReminder(newReminder("b", 200)))

	require.NoError(t, store.DeleteAllReminders(testUserID))

	assert.False(t, api.has(remindersKey(testUserID)))

	all, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestSaveReminderRejectsInvalid(t *testing.T) {
	store, api := newTestStore(t)

	for name, mutate := range map[string]func(*reminder.Reminder){
		"no id":            func(r *reminder.Reminder) { r.ID = "" },
		"no user":          func(r *reminder.Reminder) { r.UserID = "" },
		"blank message":    func(r *reminder.Reminder) { r.Message = "   " },
		"unknown timezone": func(r *reminder.Reminder) { r.Timezone = "Mars/Olympus" },
		"bad hour":         func(r *reminder.Reminder) { r.Schedule.At.Hour = 24 },
		"weekly with no weekdays": func(r *reminder.Reminder) {
			r.Schedule.Weekdays = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newReminder("abc", 100)
			mutate(r)

			assert.Error(t, store.SaveReminder(r))
			assert.False(t, api.has(remindersKey(testUserID)), "nothing should have been written")
		})
	}
}

func TestSavedReminderIsDetachedFromCaller(t *testing.T) {
	store, _ := newTestStore(t)

	r := newReminder("abc", 100)
	require.NoError(t, store.SaveReminder(r))

	// Mutating the caller's copy must not change what was stored.
	r.Message = "mutated"
	r.Schedule.Weekdays[0] = time.Friday

	got, err := store.GetReminder(testUserID, "abc")
	require.NoError(t, err)
	assert.Equal(t, "weekly report", got.Message)
	assert.Equal(t, []time.Weekday{time.Monday}, got.Schedule.Weekdays)
}

func TestRemindersAreScopedPerUser(t *testing.T) {
	store, _ := newTestStore(t)

	mine := newReminder("mine", 100)
	theirs := newReminder("theirs", 100)
	theirs.UserID = "user9999999999999999999999"

	require.NoError(t, store.SaveReminder(mine))
	require.NoError(t, store.SaveReminder(theirs))

	all, err := store.GetReminders(testUserID)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "mine", all[0].ID)

	_, err = store.GetReminder(testUserID, "theirs")
	assert.ErrorIs(t, err, reminder.ErrNotFound)
}
