package main

import (
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// Delivery is a single periodic sweep: every tick, every reminder whose
// NextRunAt has passed is delivered and moved on to its following occurrence.
// The reminder records in the KV store are the only state, which is what keeps
// deleting, editing and firing from being able to contradict each other.
//
// The obvious-looking alternative — handing each reminder to
// cluster.JobOnceScheduler and re-arming it from its own callback — does not
// work. JobOnce runs the callback while holding the cluster mutex for that key,
// so re-arming the same key from inside deadlocks against itself; its
// saveMetadata refuses to overwrite an existing key; and it deletes the job
// record as soon as the callback returns. It is built for jobs that genuinely
// happen once.

const (
	// tickInterval is how often reminders are swept for delivery. It also sets
	// the worst-case delivery lag, so it trades punctuality against the cost of
	// scanning every user's reminders.
	tickInterval = time.Minute

	// schedulerJobKey names the cluster job. cluster.Schedule guarantees only
	// one plugin instance runs the sweep at a time.
	schedulerJobKey = "deliver-reminders"

	// reminderIDLength is the length of the generated reminder IDs users type
	// into `/recurring delete`. Short enough to retype, long enough that a
	// collision within one user's list is not a practical concern.
	reminderIDLength = 16
)

// newReminderID returns an ID for a new reminder.
func newReminderID() string {
	return model.NewRandomString(reminderIDLength)
}

// startScheduler begins the periodic delivery sweep.
func (p *Plugin) startScheduler() error {
	job, err := cluster.Schedule(
		p.API,
		schedulerJobKey,
		cluster.MakeWaitForRoundedInterval(tickInterval),
		p.deliverDueReminders,
	)
	if err != nil {
		return errors.Wrap(err, "failed to schedule reminder delivery")
	}

	p.backgroundJob = job

	return nil
}

// deliverDueReminders is the scheduler tick.
func (p *Plugin) deliverDueReminders() {
	p.deliverDue(time.Now())
}

// deliverDue delivers every reminder that has come due as of now. The clock is
// a parameter so the sweep can be tested without waiting for one.
func (p *Plugin) deliverDue(now time.Time) {
	userIDs, err := p.kvstore.ListUserIDs()
	if err != nil {
		p.client.Log.Error("Failed to list users with reminders", "err", err)
		return
	}

	for _, userID := range userIDs {
		reminders, err := p.kvstore.GetReminders(userID)
		if err != nil {
			p.client.Log.Error("Failed to load reminders for delivery", "user_id", userID, "err", err)
			continue
		}

		for _, r := range reminders {
			if !r.Due(now) {
				continue
			}

			p.deliverReminder(r, now)
		}
	}
}

// deliverReminder sends one reminder and records what happened.
func (p *Plugin) deliverReminder(r *reminder.Reminder, now time.Time) {
	updated := r.Clone()
	updated.UpdatedAt = now.UnixMilli()

	if err := p.send(r); err != nil {
		updated.FailureCount++

		if updated.FailureCount >= reminder.MaxDeliveryFailures {
			// Stop rather than retry forever: the recipient is most likely
			// deactivated, and the sweep would otherwise carry this failure
			// every tick for the life of the server.
			updated.NextRunAt = 0
			p.client.Log.Error("Giving up on a reminder after repeated delivery failures",
				"user_id", r.UserID, "reminder_id", r.ID, "failures", updated.FailureCount, "err", err)
		} else {
			// Leave NextRunAt alone so the next sweep tries again.
			p.client.Log.Warn("Failed to deliver a reminder, will retry",
				"user_id", r.UserID, "reminder_id", r.ID, "failures", updated.FailureCount, "err", err)
		}
	} else {
		updated.FailureCount = 0

		// Advance from now rather than from NextRunAt: if the server was down
		// over several occurrences, the reminder fires once and then catches up
		// to the future instead of replaying every missed one.
		updated.Advance(now)
	}

	if err := p.kvstore.UpdateReminder(updated); err != nil {
		if errors.Is(err, reminder.ErrNotFound) {
			// Deleted while we were delivering it. Leave it deleted.
			return
		}
		p.client.Log.Error("Failed to record a reminder's delivery",
			"user_id", r.UserID, "reminder_id", r.ID, "err", err)
	}
}

// sendReminder delivers the reminder to its owner as a direct message from the
// plugin's bot. Plugin.send points here in production and is replaced in tests.
//
// The schedule is repeated under the message: arriving as a bare line of text
// from a bot, a reminder gives the reader no way to tell which of their
// reminders fired or when it will come round again.
func (p *Plugin) sendReminder(r *reminder.Reminder) error {
	post := &model.Post{
		Message: p.reminderMessage(r, time.Now()),
	}

	// The message is the user's own text echoed back to them in their own DM,
	// but it still goes out under the bot's name, so suppress the channel-wide
	// mention keywords rather than letting a reminder render as one.
	post.AddProp("mentionHighlightDisabled", true)

	if err := p.client.Post.DM(p.botUserID, r.UserID, post); err != nil {
		return errors.Wrap(err, "failed to send the reminder direct message")
	}

	return nil
}

// reminderMessage builds the delivered message.
//
// The schedule and the following run are repeated under the text: arriving as a
// bare line from a bot, a reminder gives the reader no way to tell which of
// their reminders fired or when it comes round again. The next run goes through
// formatRunAt so that it reads the same here as in /recurring list — the two
// drifted apart once, when this built its own timestamp.
func (p *Plugin) reminderMessage(r *reminder.Reminder, now time.Time) string {
	detail := r.Schedule.Describe()

	// r still carries the run that is firing now, so the preview needs the one
	// after it.
	if next, ok := r.NextRun(now); ok {
		preview := r.Clone()
		preview.NextRunAt = next.UnixMilli()
		detail += " · next " + p.formatRunAt(preview)
	}

	return fmt.Sprintf("⏰ **%s**\n_%s_", r.Message, detail)
}
