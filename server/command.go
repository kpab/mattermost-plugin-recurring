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

const commandHelp = `**Recurring Reminders**

Set a reminder that repeats:
* ` + "`/recurring every monday at 10:00 weekly report`" + `
* ` + "`/recurring weekdays 18:00 log off`" + `
* ` + "`/recurring daily 9am stand-up`" + `
* ` + "`/recurring monthly on the 1st 9:00 expenses`" + `
* ` + "`/recurring 毎週月曜 10:00 週次報告`" + `
* ` + "`/recurring 毎朝9時 ストレッチ`" + `

Manage them:
* ` + "`/recurring list`" + ` — show your reminders
* ` + "`/recurring delete <id>`" + ` — remove one
* ` + "`/recurring help`" + ` — show this message

Reminders are delivered as a direct message, in your own timezone.`

// registerCommand tells the server about /recurring.
func (p *Plugin) registerCommand() error {
	autocomplete := model.NewAutocompleteData(commandTrigger, "[when] [message]", "Set a reminder that repeats")
	autocomplete.AddTextArgument("When and what to remind you about, e.g. \"every monday at 10:00 weekly report\"", "[when] [message]", "")

	list := model.NewAutocompleteData("list", "", "Show your reminders")
	autocomplete.AddCommand(list)

	remove := model.NewAutocompleteData("delete", "[id]", "Delete a reminder")
	remove.AddTextArgument("The ID shown by /recurring list", "[id]", "")
	autocomplete.AddCommand(remove)

	help := model.NewAutocompleteData("help", "", "Show help")
	autocomplete.AddCommand(help)

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
	}

	return subcommandCreate, rest
}

// ExecuteCommand handles /recurring.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	action, arg := parseCommand(args.Command)

	switch action {
	case subcommandHelp:
		return ephemeral(commandHelp), nil
	case subcommandList:
		return ephemeral(p.listRemindersText(args.UserId)), nil
	case subcommandDelete:
		return ephemeral(p.deleteReminderText(args.UserId, arg)), nil
	case subcommandCreate:
		return ephemeral(p.createReminderText(args.UserId, arg)), nil
	default:
		return ephemeral(commandHelp), nil
	}
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
			return capitalise(reminder.ErrTooManyReminders.Error()) + " Delete one with `/recurring delete <id>` first."
		}
		p.client.Log.Error("Failed to save a reminder", "user_id", userID, "err", err)
		return "Something went wrong saving that reminder. Please try again."
	}

	return fmt.Sprintf("Got it — I'll remind you **%s** %s.\nNext: **%s**. ID `%s`.",
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
		return "You have no reminders. Try `/recurring help` for examples."
	}

	var b strings.Builder
	b.WriteString("| | Reminder | Schedule | Next | ID |\n|---|---|---|---|---|\n")

	for _, r := range reminders {
		state, next := "", p.formatRunAt(r)
		if r.Completed {
			state, next = ":white_check_mark:", "—"
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | `%s` |\n",
			state, escapeTableCell(r.Message), r.Schedule.Describe(), next, r.ID)
	}

	return b.String()
}

// deleteReminderText removes one reminder.
func (p *Plugin) deleteReminderText(userID, reminderID string) string {
	if reminderID == "" {
		return "Which one? Use `/recurring list` to see the IDs, then `/recurring delete <id>`."
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
func (p *Plugin) formatRunAt(r *reminder.Reminder) string {
	if r.NextRunAt == 0 {
		return "—"
	}

	return time.UnixMilli(r.NextRunAt).In(r.Location()).Format("Mon 2 Jan 15:04 MST")
}

func ephemeral(text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}

// escapeTableCell keeps a message from breaking out of the markdown table it is
// rendered in: a pipe would start a new column and a newline a new row.
func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
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
