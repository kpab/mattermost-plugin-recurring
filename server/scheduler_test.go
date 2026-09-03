package main

import (
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// These tests drive the delivery sweep end to end against a fake store and a
// fake message sink. The previous scheduler — which re-armed each reminder from
// inside its own JobOnce callback — deadlocked the first time any reminder
// fired, and nothing in the suite noticed, because the only tests were over
// string helpers. Any replacement has to keep passing these.

// fakeStore is an in-memory KVStore.
type fakeStore struct {
	reminders map[string][]*reminder.Reminder

	listErr error
	saveErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{reminders: map[string][]*reminder.Reminder{}}
}

func (s *fakeStore) GetReminders(userID string) ([]*reminder.Reminder, error) {
	out := make([]*reminder.Reminder, 0, len(s.reminders[userID]))
	for _, r := range s.reminders[userID] {
		out = append(out, r.Clone())
	}
	return out, nil
}

func (s *fakeStore) GetReminder(userID, reminderID string) (*reminder.Reminder, error) {
	for _, r := range s.reminders[userID] {
		if r.ID == reminderID {
			return r.Clone(), nil
		}
	}
	return nil, reminder.ErrNotFound
}

func (s *fakeStore) SaveReminder(r *reminder.Reminder) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	for i, existing := range s.reminders[r.UserID] {
		if existing.ID == r.ID {
			s.reminders[r.UserID][i] = r.Clone()
			return nil
		}
	}
	if len(s.reminders[r.UserID]) >= reminder.MaxRemindersPerUser {
		return reminder.ErrTooManyReminders
	}
	s.reminders[r.UserID] = append(s.reminders[r.UserID], r.Clone())
	return nil
}

func (s *fakeStore) UpdateReminder(r *reminder.Reminder) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	for i, existing := range s.reminders[r.UserID] {
		if existing.ID == r.ID {
			s.reminders[r.UserID][i] = r.Clone()
			return nil
		}
	}
	return reminder.ErrNotFound
}

func (s *fakeStore) DeleteReminder(userID, reminderID string) error {
	for i, existing := range s.reminders[userID] {
		if existing.ID == reminderID {
			s.reminders[userID] = append(s.reminders[userID][:i], s.reminders[userID][i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *fakeStore) DeleteAllReminders(userID string) error {
	delete(s.reminders, userID)
	return nil
}

func (s *fakeStore) ListUserIDs() ([]string, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]string, 0, len(s.reminders))
	for userID := range s.reminders {
		out = append(out, userID)
	}
	return out, nil
}

// silentAPI satisfies the logging calls the delivery sweep makes. Everything
// else comes from the embedded plugintest.API, which fails loudly if the code
// under test reaches for an API these tests have not thought about.
type silentAPI struct {
	plugintest.API
}

func (a *silentAPI) LogError(string, ...any) {}
func (a *silentAPI) LogWarn(string, ...any)  {}
func (a *silentAPI) LogInfo(string, ...any)  {}
func (a *silentAPI) LogDebug(string, ...any) {}

// testPlugin wires a Plugin to fakes, capturing what it would have sent.
type testPlugin struct {
	*Plugin

	store *fakeStore
	sent  []string
	fail  error
}

func newTestPlugin(t *testing.T) *testPlugin {
	t.Helper()

	store := newFakeStore()
	tp := &testPlugin{
		Plugin: &Plugin{
			kvstore: store,
			client:  pluginapi.NewClient(&silentAPI{}, nil),
		},
		store: store,
	}

	// sendReminder is replaced wholesale so the tests do not need a live
	// Mattermost API; delivery success or failure is what the sweep branches on.
	tp.Plugin.send = func(r *reminder.Reminder) error {
		if tp.fail != nil {
			return tp.fail
		}
		tp.sent = append(tp.sent, r.Message)
		return nil
	}

	return tp
}

func testReminder(userID, id string, at reminder.TimeOfDay, nextRunAt int64) *reminder.Reminder {
	return &reminder.Reminder{
		ID:        id,
		UserID:    userID,
		Message:   "stand-up",
		Schedule:  reminder.Schedule{Kind: reminder.KindDaily, At: at},
		Timezone:  "Asia/Tokyo",
		NextRunAt: nextRunAt,
		CreatedAt: 1,
	}
}

func TestDeliverDueRemindersFiresAndAdvances(t *testing.T) {
	tp := newTestPlugin(t)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	now := time.Date(2026, time.September, 3, 9, 0, 30, 0, tokyo)
	due := time.Date(2026, time.September, 3, 9, 0, 0, 0, tokyo)

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, due.UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	tp.deliverDue(now)

	assert.Equal(t, []string{"stand-up"}, tp.sent)

	stored, err := tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)
	assert.Equal(t,
		time.Date(2026, time.September, 4, 9, 0, 0, 0, tokyo).UnixMilli(), stored.NextRunAt,
		"the reminder must move on to tomorrow")
}

// The whole point of the plugin: it has to keep firing, week after week.
func TestDeliverDueRemindersRepeats(t *testing.T) {
	tp := newTestPlugin(t)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	start := time.Date(2026, time.September, 3, 9, 0, 0, 0, tokyo)
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, start.UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	// Sweep once a day for a week.
	for day := range 7 {
		now := start.AddDate(0, 0, day).Add(30 * time.Second)
		tp.deliverDue(now)
	}

	assert.Len(t, tp.sent, 7, "a daily reminder must fire once a day, every day")
}

func TestDeliverDueRemindersSkipsNotYetDue(t *testing.T) {
	tp := newTestPlugin(t)

	now := time.Now()
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, now.Add(time.Hour).UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	tp.deliverDue(now)

	assert.Empty(t, tp.sent)
}

func TestDeliverDueRemindersSkipsCompleted(t *testing.T) {
	tp := newTestPlugin(t)

	now := time.Now()
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, now.Add(-time.Hour).UnixMilli())
	r.Completed = true
	require.NoError(t, tp.store.SaveReminder(r))

	tp.deliverDue(now)

	assert.Empty(t, tp.sent, "a completed reminder must not fire")
}

// Missing several occurrences while the server was down must produce one
// message, not one per missed occurrence.
func TestDeliverDueRemindersCatchesUpWithoutReplaying(t *testing.T) {
	tp := newTestPlugin(t)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	missed := time.Date(2026, time.September, 1, 9, 0, 0, 0, tokyo)
	now := time.Date(2026, time.September, 5, 12, 0, 0, 0, tokyo)

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, missed.UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	tp.deliverDue(now)

	assert.Len(t, tp.sent, 1, "four missed days must produce one message, not four")

	stored, err := tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)
	assert.Equal(t,
		time.Date(2026, time.September, 6, 9, 0, 0, 0, tokyo).UnixMilli(), stored.NextRunAt,
		"the reminder must catch up to the future")
}

func TestDeliverDueRemindersRetriesThenGivesUp(t *testing.T) {
	tp := newTestPlugin(t)
	tp.fail = assert.AnError

	now := time.Now()
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, now.Add(-time.Minute).UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	for range reminder.MaxDeliveryFailures - 1 {
		tp.deliverDue(now)

		stored, err := tp.store.GetReminder("user1", "r1")
		require.NoError(t, err)
		assert.NotZero(t, stored.NextRunAt, "should still be queued for retry")
	}

	tp.deliverDue(now)

	stored, err := tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)
	assert.Zero(t, stored.NextRunAt,
		"after repeated failures the reminder must stop being retried forever")
}

func TestDeliverySuccessResetsFailureCount(t *testing.T) {
	tp := newTestPlugin(t)
	tp.fail = assert.AnError

	now := time.Now()
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, now.Add(-time.Minute).UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	tp.deliverDue(now)

	stored, err := tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)
	require.Equal(t, 1, stored.FailureCount)

	tp.fail = nil
	tp.deliverDue(now)

	stored, err = tp.store.GetReminder("user1", "r1")
	require.NoError(t, err)
	assert.Zero(t, stored.FailureCount)
}

// A reminder deleted while it is being delivered must stay deleted. Writing the
// post-delivery state back with an upsert would resurrect it.
func TestDeliveryDoesNotResurrectADeletedReminder(t *testing.T) {
	tp := newTestPlugin(t)

	now := time.Now()
	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, now.Add(-time.Minute).UnixMilli())
	require.NoError(t, tp.store.SaveReminder(r))

	// Delete it midway through delivery.
	tp.Plugin.send = func(_ *reminder.Reminder) error {
		return tp.store.DeleteReminder("user1", "r1")
	}

	tp.deliverDue(now)

	_, err := tp.store.GetReminder("user1", "r1")
	assert.ErrorIs(t, err, reminder.ErrNotFound, "the deleted reminder must not come back")
}

func TestDeliverDueRemindersHandlesSeveralUsers(t *testing.T) {
	tp := newTestPlugin(t)

	now := time.Now()
	due := now.Add(-time.Minute).UnixMilli()

	require.NoError(t, tp.store.SaveReminder(testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, due)))
	require.NoError(t, tp.store.SaveReminder(testReminder("user2", "r2", reminder.TimeOfDay{Hour: 9}, due)))

	tp.deliverDue(now)

	assert.Len(t, tp.sent, 2)
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

// Reminder IDs end up in KV keys and in command arguments, so they must stay
// within the server's key limit and contain no separator characters.
func TestReminderIDIsKeySafe(t *testing.T) {
	const kvKeyLimit = model.KeyValueKeyMaxRunes

	id := newReminderID()
	assert.NotContains(t, id, " ")
	assert.NotContains(t, id, "/")
	assert.LessOrEqual(t, len("reminders-")+len(model.NewId()), kvKeyLimit)
}
