package kvstore

import (
	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// KVStore is how the rest of the plugin reaches persistent storage. Going
// through an interface keeps the callers testable and keeps the choice of keys
// in one place.
type KVStore interface {
	// GetReminders returns every reminder owned by the user, oldest first.
	GetReminders(userID string) ([]*reminder.Reminder, error)
	// GetReminder returns a single reminder, or reminder.ErrNotFound.
	GetReminder(userID, reminderID string) (*reminder.Reminder, error)
	// SaveReminder inserts or replaces a reminder, rejecting the insert if the
	// user is already at reminder.MaxRemindersPerUser.
	SaveReminder(r *reminder.Reminder) error
	// UpdateReminder replaces an existing reminder, returning
	// reminder.ErrNotFound if it has since been deleted.
	UpdateReminder(r *reminder.Reminder) error
	// DeleteReminder removes a reminder. Removing an absent one is not an error.
	DeleteReminder(userID, reminderID string) error
	// DeleteAllReminders removes every reminder owned by the user.
	DeleteAllReminders(userID string) error
	// ListUserIDs returns the ID of every user who owns at least one reminder.
	// Used on activation to re-queue reminders the scheduler has lost track of.
	ListUserIDs() ([]string, error)
}
