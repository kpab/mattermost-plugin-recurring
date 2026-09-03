package main

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/pkg/errors"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

const commandTrigger = "recurring"

const commandHelp = "**Recurring Reminders** — reminders that repeat. Say when, then what.\n" + `
Most used:
* ` + "`/recurring daily 9:00 stand-up`" + `
* ` + "`/recurring weekdays 18:00 log off`" + `
* ` + "`/recurring every monday at 10:00 weekly report`" + `
* ` + "`/recurring monthly on the 1st 9:00 expenses`" + `

Times can be ` + "`10:00`" + `, ` + "`9am`" + `, ` + "`6:30pm`" + `, or ` + "`at 9`" + `.

Manage them:
* ` + "`/recurring list`" + ` — show your reminders
* ` + "`/recurring pause <id>`" + ` / ` + "`/recurring resume <id>`" + ` — stop and start one
* ` + "`/recurring delete <id>`" + ` — remove one

Japanese input works too:
* ` + "`/recurring 毎朝9時 ストレッチ`" + `
* ` + "`/recurring 毎週月曜 10:00 週次報告`" + `
_(replies are in English for now)_`

// registerCommand tells the server about /recurring.
func (p *Plugin) registerCommand() error {
	autocomplete := model.NewAutocompleteData(commandTrigger, "[when] [message]", "Set a reminder that repeats")
	autocomplete.AddTextArgument("e.g. every monday at 10:00 weekly report", "[when] [message]", "")

	autocomplete.AddCommand(model.NewAutocompleteData("list", "", "Show your reminders"))

	remove := model.NewAutocompleteData("delete", "[id]", "Delete a reminder")
	remove.AddTextArgument("The ID shown by /recurring list", "[id]", "")
	autocomplete.AddCommand(remove)

	pause := model.NewAutocompleteData("pause", "[id]", "Stop a reminder without deleting it")
	pause.AddTextArgument("The ID shown by /recurring list", "[id]", "")
	autocomplete.AddCommand(pause)

	resume := model.NewAutocompleteData("resume", "[id]", "Start a paused reminder again")
	resume.AddTextArgument("The ID shown by /recurring list", "[id]", "")
	autocomplete.AddCommand(resume)

	autocomplete.AddCommand(model.NewAutocompleteData("help", "", "Show help"))

	if err := p.client.SlashCommand.Register(&model.Command{
		Trigger:          commandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Set a reminder that repeats",
		AutoCompleteHint: "[when] [message]",
		AutocompleteData: autocomplete,
	}); err != nil {
		return errors.Wrap(err, "failed to register the /recurring command")
	}

	return nil
}

// subcommand is what the user asked /recurring to do.
type subcommand int

const (
	// subcommandCreate is the default: everything that is not a known verb is
	// treated as the description of a new reminder.
	subcommandCreate subcommand = iota
	subcommandList
	subcommandDelete
	subcommandPause
	subcommandResume
	subcommandHelp
)

// parseCommand splits raw command text into a subcommand and its argument.
// Only an exact verb counts, so "delete the milk every day" still creates a
// reminder rather than being read as a deletion.
func parseCommand(raw string) (subcommand, string) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/"+commandTrigger))
	rest = strings.TrimSpace(rest)

	verb, arg, _ := strings.Cut(rest, " ")
	arg = strings.TrimSpace(arg)

	switch strings.ToLower(verb) {
	case "", "help":
		if arg == "" {
			return subcommandHelp, ""
		}
	case "list":
		if arg == "" {
			return subcommandList, ""
		}
	case "delete", "remove", "rm":
		return subcommandDelete, arg
	case "pause", "stop":
		return subcommandPause, arg
	case "resume", "start":
		return subcommandResume, arg
	}

	return subcommandCreate, rest
}

// ExecuteCommand handles /recurring.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	action, arg := parseCommand(args.Command)

	switch action {
	case subcommandHelp:
		return ephemeral(p.helpText(args.UserId)), nil
	case subcommandList:
		return ephemeral(p.listRemindersText(args.UserId)), nil
	case subcommandDelete:
		return ephemeral(p.deleteReminderText(args.UserId, arg)), nil
	case subcommandPause:
		return ephemeral(p.setPausedText(args.UserId, arg, true)), nil
	case subcommandResume:
		return ephemeral(p.setPausedText(args.UserId, arg, false)), nil
	case subcommandCreate:
		return ephemeral(p.createReminderText(args.UserId, arg)), nil
	default:
		return ephemeral(p.helpText(args.UserId)), nil
	}
}

// helpText is the command help, with the reader's own timezone filled in so
// they can see up front which clock their reminders will follow.
func (p *Plugin) helpText(userID string) string {
	timezone, err := p.userTimezone(userID)
	if err != nil {
		return commandHelp + "\n\nReminders arrive as a direct message, in your own timezone."
	}

	return fmt.Sprintf("%s\n\nReminders arrive as a direct message, in your own timezone (%s).", commandHelp, timezone)
}

// createReminderText parses the input, stores the reminder and queues it.
func (p *Plugin) createReminderText(userID, input string) string {
	schedule, message, err := reminder.ParseRecurring(input)
	if err != nil {
		return fmt.Sprintf("%s\n\nTry `/recurring help` for examples.", capitalise(err.Error()))
	}

	timezone, err := p.userTimezone(userID)
	if err != nil {
		p.client.Log.Warn("Failed to read a user's timezone, falling back to UTC", "user_id", userID, "err", err)
		timezone = "UTC"
	}

	now := time.Now()
	r := &reminder.Reminder{
		ID:        newReminderID(),
		UserID:    userID,
		Message:   message,
		Schedule:  schedule,
		Timezone:  timezone,
		CreatedAt: now.UnixMilli(),
		UpdatedAt: now.UnixMilli(),
	}

	if !r.Advance(now) {
		return "That reminder would never fire."
	}

	if err := p.kvstore.SaveReminder(r); err != nil {
		if errors.Is(err, reminder.ErrTooManyReminders) {
			return capitalise(reminder.ErrTooManyReminders.Error()) +
				". Delete one with `/recurring delete <id>`, then try again."
		}
		p.client.Log.Error("Failed to save a reminder", "user_id", userID, "err", err)
		return "Something went wrong saving that reminder. Please try again."
	}

	return fmt.Sprintf("Got it — I'll remind you **%s** %s.\nNext: **%s**\nTo cancel: `/recurring delete %s`",
		r.Message, r.Schedule.Describe(), p.formatRunAt(r), r.ID)
}

// listRemindersText renders the user's reminders.
func (p *Plugin) listRemindersText(userID string) string {
	reminders, err := p.kvstore.GetReminders(userID)
	if err != nil {
		p.client.Log.Error("Failed to list reminders", "user_id", userID, "err", err)
		return "Something went wrong reading your reminders. Please try again."
	}

	if len(reminders) == 0 {
		return "You have no reminders yet. Create one like this:\n" +
			"`/recurring every monday at 10:00 weekly report`\n" +
			"More examples: `/recurring help`"
	}

	// Rendered as a list rather than a table: a table needs an ID column wide
	// enough for a 16 character string, and any real reminder text pushes it
	// past the width of a channel on a phone.
	var b strings.Builder
	fmt.Fprintf(&b, "**Your reminders** (%d)\n", len(reminders))

	for i, r := range reminders {
		state := ""
		if r.Paused {
			state = " _(paused)_"
		}

		fmt.Fprintf(&b, "\n**%d.** %s%s\n%s · next %s\n`/recurring delete %s`\n",
			i+1, escapeInline(r.Message), state, r.Schedule.Describe(), p.formatRunAt(r), r.ID)
	}

	return b.String()
}

// deleteReminderText removes one reminder.
func (p *Plugin) deleteReminderText(userID, reminderID string) string {
	if reminderID == "" {
		return "Which one? Use `/recurring list` to see your reminders — each one comes with the command to remove it."
	}

	r, err := p.kvstore.GetReminder(userID, reminderID)
	if err != nil {
		if errors.Is(err, reminder.ErrNotFound) {
			return fmt.Sprintf("No reminder with ID `%s`. Use `/recurring list` to see yours.", reminderID)
		}
		p.client.Log.Error("Failed to look up a reminder for deletion", "user_id", userID, "err", err)
		return "Something went wrong. Please try again."
	}

	if err := p.kvstore.DeleteReminder(userID, reminderID); err != nil {
		p.client.Log.Error("Failed to delete a reminder", "user_id", userID, "reminder_id", reminderID, "err", err)
		return "Something went wrong deleting that reminder. Please try again."
	}

	return fmt.Sprintf("Deleted **%s**.", r.Message)
}

// setPausedText pauses or resumes a reminder.
func (p *Plugin) setPausedText(userID, reminderID string, paused bool) string {
	verb := "resume"
	if paused {
		verb = "pause"
	}

	if reminderID == "" {
		return fmt.Sprintf("Which one? Use `/recurring list` to see your reminders, then `/recurring %s <id>`.", verb)
	}

	r, err := p.kvstore.GetReminder(userID, reminderID)
	if err != nil {
		if errors.Is(err, reminder.ErrNotFound) {
			return fmt.Sprintf("No reminder with ID `%s`. Use `/recurring list` to see yours.", reminderID)
		}
		p.client.Log.Error("Failed to look up a reminder", "user_id", userID, "err", err)
		return "Something went wrong. Please try again."
	}

	if r.Paused == paused {
		if paused {
			return fmt.Sprintf("**%s** is already paused. Resume it with `/recurring resume %s`.", escapeInline(r.Message), r.ID)
		}
		return fmt.Sprintf("**%s** is already running. Next: **%s**", escapeInline(r.Message), p.formatRunAt(r))
	}

	now := time.Now()
	r.Paused = paused
	r.UpdatedAt = now.UnixMilli()

	if !paused {
		// Its next run is in the past by now, so work out the following one.
		r.Advance(now)
	}

	if err := p.kvstore.UpdateReminder(r); err != nil {
		p.client.Log.Error("Failed to pause or resume a reminder", "user_id", userID, "reminder_id", reminderID, "err", err)
		return "Something went wrong. Please try again."
	}

	if paused {
		return fmt.Sprintf("Paused **%s**. Resume it with `/recurring resume %s`.", escapeInline(r.Message), r.ID)
	}

	return fmt.Sprintf("Resumed **%s**. Next: **%s**", escapeInline(r.Message), p.formatRunAt(r))
}

// userTimezone returns the IANA timezone the user has set in Mattermost.
func (p *Plugin) userTimezone(userID string) (string, error) {
	user, err := p.client.User.Get(userID)
	if err != nil {
		return "", errors.Wrap(err, "failed to get user")
	}

	timezone := user.GetPreferredTimezone()
	if timezone == "" {
		return "UTC", nil
	}

	if _, err := time.LoadLocation(timezone); err != nil {
		return "", errors.Wrapf(err, "user has an unusable timezone %q", timezone)
	}

	return timezone, nil
}

// formatRunAt renders a reminder's next firing in its own timezone.
//
// Near dates are named rather than dated: "Today at 09:00" is read faster than
// "Thu, 4 Sep at 09:00", and the next firing is usually near. The zone
// abbreviation always stays — it is how a user spots that a reminder is an hour
// off after a daylight-saving change.
func (p *Plugin) formatRunAt(r *reminder.Reminder) string {
	if r.Paused {
		return "paused"
	}
	if r.NextRunAt == 0 {
		return "never again"
	}

	loc := r.Location()
	next := time.UnixMilli(r.NextRunAt).In(loc)
	clock := next.Format("15:04 MST")

	today := time.Now().In(loc)
	daysAway := daysBetween(today, next)

	switch {
	case daysAway == 0:
		return "Today at " + clock
	case daysAway == 1:
		return "Tomorrow at " + clock
	case daysAway < 7:
		return next.Format("Monday") + " at " + clock
	case next.Year() == today.Year():
		return next.Format("Mon, 2 Jan") + " at " + clock
	default:
		return next.Format("Mon, 2 Jan 2006") + " at " + clock
	}
}

// daysBetween counts calendar days from one local time to another.
func daysBetween(from, to time.Time) int {
	fromDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)

	return int(toDay.Sub(fromDay).Hours() / 24)
}

func ephemeral(text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}

// escapeInline flattens a message onto one line so a reminder containing
// newlines cannot break the surrounding list.
func escapeInline(s string) string {
	// CRLF first, so a Windows line ending collapses to one space rather than two.
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")

	return s
}

// capitalise upper-cases the first letter, so error strings read as sentences.
// It works in runes: slicing the first byte would corrupt a message that starts
// with a multi-byte character.
func capitalise(s string) string {
	if s == "" {
		return s
	}

	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}
