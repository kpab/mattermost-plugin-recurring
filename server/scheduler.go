package main

import (
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// Reminders are handed to cluster.JobOnceScheduler one firing at a time: when a
// reminder fires we work out when it should next go off and queue that. The
// scheduler persists each pending job and guarantees exactly one plugin
// instance runs it, which is what makes this safe on a cluster and across
// restarts. cluster.Schedule is not usable here because it only understands
// fixed intervals, and "every Monday at 10:00" is not one.

const (
	// reminderIDLength keeps job keys inside the server's 50 character KV key
	// limit. A key is "<userID>_<reminderID>" (26 + 1 + 16 = 43) and the
	// scheduler adds a "once_" or "mutex_" prefix of its own.
	reminderIDLength = 16

	jobKeySeparator = "_"
)

// newReminderID returns an ID short enough to fit in a job key.
func newReminderID() string {
	return model.NewRandomString(reminderIDLength)
}

// jobKey identifies a reminder's pending job. The user ID is part of the key so
// the callback can find the reminder again without unpacking job props, whose
// concrete type differs between a job scheduled in this process and one
// restored from the KV store.
func jobKey(userID, reminderID string) string {
	return userID + jobKeySeparator + reminderID
}

func parseJobKey(key string) (userID, reminderID string, ok bool) {
	userID, reminderID, found := strings.Cut(key, jobKeySeparator)
	if !found || userID == "" || reminderID == "" {
		return "", "", false
	}

	return userID, reminderID, true
}

// startScheduler wires up the reminder callback and starts the scheduler.
// Starting it re-queues every job already persisted in the KV store, so
// reminders survive a restart.
func (p *Plugin) startScheduler() error {
	scheduler := cluster.GetJobOnceScheduler(p.API)

	if err := scheduler.SetCallback(p.handleReminderFired); err != nil {
		return errors.Wrap(err, "failed to set the reminder callback")
	}

	if err := scheduler.Start(); err != nil {
		return errors.Wrap(err, "failed to start the reminder scheduler")
	}

	p.scheduler = scheduler

	// Best effort: a reminder whose job went missing would otherwise stay
	// silent forever, so re-queue those. Failing here is not worth refusing to
	// activate over.
	if err := p.requeueOrphanedReminders(); err != nil {
		p.client.Log.Warn("Failed to re-queue reminders missing a scheduled job", "err", err)
	}

	return nil
}

// scheduleReminder queues the reminder's next firing, or cancels any pending
// job if it is not going to fire again.
func (p *Plugin) scheduleReminder(r *reminder.Reminder) error {
	key := jobKey(r.UserID, r.ID)

	if r.Completed || r.NextRunAt == 0 {
		p.scheduler.Cancel(key)
		return nil
	}

	if _, err := p.scheduler.ScheduleOnce(key, time.UnixMilli(r.NextRunAt), nil); err != nil {
		return errors.Wrap(err, "failed to schedule reminder")
	}

	return nil
}

// cancelReminder drops any pending job for a reminder.
func (p *Plugin) cancelReminder(userID, reminderID string) {
	p.scheduler.Cancel(jobKey(userID, reminderID))
}

// handleReminderFired runs when a reminder comes due. It delivers the message
// and then queues the following occurrence.
func (p *Plugin) handleReminderFired(key string, _ any) {
	userID, reminderID, ok := parseJobKey(key)
	if !ok {
		p.client.Log.Warn("Ignoring a reminder job with an unrecognised key", "key", key)
		return
	}

	r, err := p.kvstore.GetReminder(userID, reminderID)
	if err != nil {
		if errors.Is(err, reminder.ErrNotFound) {
			// Deleted between being queued and firing. Nothing to do.
			return
		}
		p.client.Log.Error("Failed to load a reminder that came due", "user_id", userID, "reminder_id", reminderID, "err", err)
		return
	}

	if r.Completed {
		return
	}

	if err := p.sendReminder(r); err != nil {
		p.client.Log.Error("Failed to deliver a reminder", "user_id", userID, "reminder_id", reminderID, "err", err)
		// Fall through and reschedule anyway: dropping the whole series
		// because one delivery failed would be worse than missing one message.
	}

	now := time.Now()
	r.Advance(now)
	r.UpdatedAt = now.UnixMilli()

	if err := p.kvstore.SaveReminder(r); err != nil {
		p.client.Log.Error("Failed to record a reminder's next run", "user_id", userID, "reminder_id", reminderID, "err", err)
		return
	}

	if err := p.scheduleReminder(r); err != nil {
		p.client.Log.Error("Failed to queue a reminder's next run", "user_id", userID, "reminder_id", reminderID, "err", err)
	}
}

// sendReminder delivers the reminder to its owner as a direct message from the
// plugin's bot.
func (p *Plugin) sendReminder(r *reminder.Reminder) error {
	post := &model.Post{
		Message: r.Message,
	}

	if err := p.client.Post.DM(p.botUserID, r.UserID, post); err != nil {
		return errors.Wrap(err, "failed to send the reminder direct message")
	}

	return nil
}

// requeueOrphanedReminders queues any reminder that should be pending but has
// no job behind it. This can happen if the plugin died between saving a
// reminder and scheduling it, and without this the reminder would never fire
// again.
func (p *Plugin) requeueOrphanedReminders() error {
	jobs, err := p.scheduler.ListScheduledJobs()
	if err != nil {
		return errors.Wrap(err, "failed to list scheduled jobs")
	}

	scheduled := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		scheduled[job.Key] = true
	}

	userIDs, err := p.kvstore.ListUserIDs()
	if err != nil {
		return errors.Wrap(err, "failed to list users with reminders")
	}

	for _, userID := range userIDs {
		reminders, err := p.kvstore.GetReminders(userID)
		if err != nil {
			p.client.Log.Warn("Failed to load reminders while re-queueing", "user_id", userID, "err", err)
			continue
		}

		for _, r := range reminders {
			if r.Completed || r.NextRunAt == 0 || scheduled[jobKey(r.UserID, r.ID)] {
				continue
			}

			if err := p.scheduleReminder(r); err != nil {
				p.client.Log.Warn("Failed to re-queue a reminder", "user_id", userID, "reminder_id", r.ID, "err", err)
			}
		}
	}

	return nil
}
