package kvstore

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// remindersKeyPrefix namespaces the per-user reminder lists. Mattermost caps KV
// keys at 50 characters; this prefix plus a 26 character user ID stays well
// under that.
const remindersKeyPrefix = "reminders-"

func remindersKey(userID string) string {
	return remindersKeyPrefix + userID
}

// A user's reminders are stored as a single JSON list rather than one record
// each, because every read path we have wants the whole list (rendering the
// RHS, restoring schedules on activation) and a user holds few enough of them
// for that to stay cheap. Writes go through SetAtomicWithRetries so two
// concurrent updates cannot clobber each other.

// GetReminders returns every reminder owned by the user, oldest first. A user
// with no reminders yields an empty slice rather than an error.
func (kv Client) GetReminders(userID string) ([]*reminder.Reminder, error) {
	var reminders []*reminder.Reminder
	if err := kv.client.KV.Get(remindersKey(userID), &reminders); err != nil {
		return nil, errors.Wrap(err, "failed to get reminders")
	}

	sortReminders(reminders)

	return reminders, nil
}

// GetReminder returns a single reminder, or reminder.ErrNotFound.
func (kv Client) GetReminder(userID, reminderID string) (*reminder.Reminder, error) {
	reminders, err := kv.GetReminders(userID)
	if err != nil {
		return nil, err
	}

	for _, r := range reminders {
		if r.ID == reminderID {
			return r, nil
		}
	}

	return nil, reminder.ErrNotFound
}

// SaveReminder inserts the reminder, or replaces the existing one with the same
// ID. The stored copy is detached from the caller's.
func (kv Client) SaveReminder(r *reminder.Reminder) error {
	if err := r.Validate(); err != nil {
		return errors.Wrap(err, "refusing to save an invalid reminder")
	}

	toSave := r.Clone()

	return kv.updateReminders(r.UserID, func(reminders []*reminder.Reminder) ([]*reminder.Reminder, error) {
		for i, existing := range reminders {
			if existing.ID == toSave.ID {
				reminders[i] = toSave
				return reminders, nil
			}
		}

		if len(reminders) >= reminder.MaxRemindersPerUser {
			return nil, reminder.ErrTooManyReminders
		}

		return append(reminders, toSave), nil
	})
}

// UpdateReminder replaces an existing reminder, returning reminder.ErrNotFound
// if it is already gone.
//
// Delivery uses this rather than SaveReminder: a reminder the user deleted
// while it was being delivered must stay deleted, and an upsert would quietly
// resurrect it with a fresh NextRunAt.
func (kv Client) UpdateReminder(r *reminder.Reminder) error {
	if err := r.Validate(); err != nil {
		return errors.Wrap(err, "refusing to save an invalid reminder")
	}

	toSave := r.Clone()

	return kv.updateReminders(r.UserID, func(reminders []*reminder.Reminder) ([]*reminder.Reminder, error) {
		for i, existing := range reminders {
			if existing.ID == toSave.ID {
				reminders[i] = toSave
				return reminders, nil
			}
		}

		return nil, reminder.ErrNotFound
	})
}

// DeleteReminder removes a reminder. Deleting one that is not there is not an
// error: the caller wanted it gone, and it is.
func (kv Client) DeleteReminder(userID, reminderID string) error {
	return kv.updateReminders(userID, func(reminders []*reminder.Reminder) ([]*reminder.Reminder, error) {
		for i, existing := range reminders {
			if existing.ID == reminderID {
				return append(reminders[:i], reminders[i+1:]...), nil
			}
		}
		return reminders, nil
	})
}

// DeleteAllReminders removes every reminder owned by the user, for when they
// are deactivated or ask us to forget them.
func (kv Client) DeleteAllReminders(userID string) error {
	if err := kv.client.KV.Delete(remindersKey(userID)); err != nil {
		return errors.Wrap(err, "failed to delete reminders")
	}
	return nil
}

// listKeysPerPage is how many KV keys ListUserIDs pulls per round trip.
const listKeysPerPage = 100

// ListUserIDs returns the ID of every user who owns at least one reminder.
func (kv Client) ListUserIDs() ([]string, error) {
	var userIDs []string

	for page := 0; ; page++ {
		// Deliberately not using pluginapi's WithPrefix option: it filters
		// after the page has been fetched, so a page holding only other keys
		// (the scheduler's own "once_" records, for instance) would come back
		// empty and be mistaken for the end of the list.
		keys, err := kv.client.KV.ListKeys(page, listKeysPerPage)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list reminder keys")
		}

		for _, key := range keys {
			if strings.HasPrefix(key, remindersKeyPrefix) {
				userIDs = append(userIDs, strings.TrimPrefix(key, remindersKeyPrefix))
			}
		}

		if len(keys) < listKeysPerPage {
			break
		}
	}

	return userIDs, nil
}

// updateReminders applies mutate to the user's stored list under an atomic
// compare-and-set, retrying if another writer got there first.
func (kv Client) updateReminders(userID string, mutate func([]*reminder.Reminder) ([]*reminder.Reminder, error)) error {
	err := kv.client.KV.SetAtomicWithRetries(remindersKey(userID), func(oldValue []byte) (any, error) {
		reminders, err := decodeReminders(oldValue)
		if err != nil {
			return nil, err
		}

		updated, err := mutate(reminders)
		if err != nil {
			return nil, err
		}

		if len(updated) == 0 {
			// Storing an empty list would leave a row behind for every user who
			// ever tried the plugin; a nil value deletes the key instead.
			return nil, nil
		}

		sortReminders(updated)

		return updated, nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to update reminders")
	}

	return nil
}

// decodeReminders reads the stored JSON list. A missing or empty value means
// the user simply has no reminders yet.
func decodeReminders(raw []byte) ([]*reminder.Reminder, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var reminders []*reminder.Reminder
	if err := json.Unmarshal(raw, &reminders); err != nil {
		return nil, errors.Wrap(err, "failed to decode stored reminders")
	}

	return reminders, nil
}

func sortReminders(reminders []*reminder.Reminder) {
	sort.SliceStable(reminders, func(i, j int) bool {
		if reminders[i].CreatedAt != reminders[j].CreatedAt {
			return reminders[i].CreatedAt < reminders[j].CreatedAt
		}
		return reminders[i].ID < reminders[j].ID
	})
}
