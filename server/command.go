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

const commandHelpEN = "**Recurring Reminders** — reminders that repeat. Say when, then what.\n" + `
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

Japanese input also works:
* ` + "`/recurring 毎朝9時 ストレッチ`" + ``

const commandHelpJA = "**繰り返しリマインダー** — 周期・時刻・本文の順に書きます。\n" + `
よく使う形:
* ` + "`/recurring 毎日 9:00 朝会`" + `
* ` + "`/recurring 平日 18:00 退勤`" + `
* ` + "`/recurring 毎週月曜 10:00 週次報告`" + `
* ` + "`/recurring 毎月1日 9:00 経費精算`" + `

時刻は ` + "`10:00`" + `、` + "`9時`" + `、` + "`9時半`" + `、` + "`18:30`" + ` のように書けます。

管理:
* ` + "`/recurring list`" + ` — 一覧を表示(ボタンで停止・削除できます)
* ` + "`/recurring pause <id>`" + ` / ` + "`/recurring resume <id>`" + ` — 停止と再開
* ` + "`/recurring delete <id>`" + ` — 削除

英語でも書けます:
* ` + "`/recurring every monday at 10:00 weekly report`" + ``

// buildAutocomplete describes /recurring to the autocomplete UI.
//
// The root carries subcommands only. Mattermost rejects a definition that has
// both arguments and subcommands on the same node — and rejects it by failing
// command registration, which fails OnActivate, which stops the plugin from
// starting at all. Creating a reminder therefore has no argument hint here; it
// is whatever does not match a subcommand, and the hint on the Command itself
// carries the example.
func buildAutocomplete() *model.AutocompleteData {
	autocomplete := model.NewAutocompleteData(commandTrigger, "[when] [message]", "Set a reminder that repeats")

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

	return autocomplete
}

// registerCommand tells the server about /recurring.
func (p *Plugin) registerCommand() error {
	if err := p.client.SlashCommand.Register(&model.Command{
		Trigger:          commandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Set a reminder that repeats — daily, weekly, or monthly",
		AutoCompleteHint: "daily 9:00 stand-up",
		AutocompleteData: buildAutocomplete(),
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
		return p.listRemindersResponse(args.UserId, args.ChannelId), nil
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
	lang := p.langFor(userID)

	help := commandHelpEN
	if lang == reminder.LangJA {
		help = commandHelpJA
	}

	timezone, err := p.userTimezone(userID)
	if err != nil {
		return fmt.Sprintf(msg(lang, "help.timezoneUnknown"), help)
	}

	return fmt.Sprintf(msg(lang, "help.timezone"), help, timezone)
}

// createReminderText parses the input, stores the reminder and queues it.
func (p *Plugin) createReminderText(userID, input string) string {
	lang := p.langFor(userID)

	schedule, message, err := reminder.ParseRecurring(input)
	if err != nil {
		return fmt.Sprintf(msg(lang, "parse.retry"), parseErrorText(lang, err))
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
		return msg(lang, "created.never")
	}

	if err := p.kvstore.SaveReminder(r); err != nil {
		if errors.Is(err, reminder.ErrTooManyReminders) {
			return capitalise(reminder.ErrTooManyReminders.Error()) + msg(lang, "created.tooMany")
		}
		p.client.Log.Error("Failed to save a reminder", "user_id", userID, "err", err)
		return msg(lang, "created.failed")
	}

	return fmt.Sprintf(msg(lang, "created"),
		r.Message, r.Schedule.Describe(lang), p.formatRunAt(r, lang), r.ID)
}

// parseErrorText translates the parse failures worth translating.
//
// The three sentinels cover every way of getting the shape of the input wrong,
// which is what a first-time user runs into. The remaining errors are built
// around the offending text ("`25:00` isn't a valid time") and stay in English:
// translating them means restructuring them into typed values, and they are
// only reached by someone who already got the shape right.
func parseErrorText(lang reminder.Lang, err error) string {
	switch {
	case errors.Is(err, reminder.ErrNoSchedule):
		return msg(lang, "parse.noSchedule")
	case errors.Is(err, reminder.ErrNoTime):
		return msg(lang, "parse.noTime")
	case errors.Is(err, reminder.ErrNoMessage):
		return msg(lang, "parse.noMessage")
	default:
		return capitalise(err.Error())
	}
}

// listRemindersResponse sends the user's reminders to their bot DM, one
// attachment each so that every reminder carries its own buttons.
//
// Buttons rather than printed IDs, because the mobile app runs no plugin UI:
// there is no sidebar there to click, and copying a 16 character ID on a phone
// to paste into a second command is not a workflow anyone will use twice.
//
// The list goes to the DM rather than coming back as an ephemeral reply because
// an ephemeral post vanishes when it is dismissed or the client reloads, taking
// the buttons with it. In the DM it stays, next to the reminders themselves.
func (p *Plugin) listRemindersResponse(userID, channelID string) *model.CommandResponse {
	lang := p.langFor(userID)

	reminders, err := p.kvstore.GetReminders(userID)
	if err != nil {
		p.client.Log.Error("Failed to list reminders", "user_id", userID, "err", err)
		return ephemeral(msg(lang, "list.failed"))
	}

	if len(reminders) == 0 {
		// Nothing to keep around, so an ephemeral reply is the lighter answer.
		return ephemeral(msg(lang, "list.empty"))
	}

	attachments := make([]*model.SlackAttachment, 0, len(reminders))
	for _, r := range reminders {
		detail := r.Schedule.Describe(lang)
		if r.Paused {
			detail += " · " + msg(lang, "state.paused")
		} else {
			detail += " · " + fmt.Sprintf(msg(lang, "next"), p.formatRunAt(r, lang))
		}

		attachments = append(attachments, &model.SlackAttachment{
			Text:    "**" + escapeInline(r.Message) + "**\n" + detail,
			Actions: p.listActions(r, lang),
		})
	}

	post := &model.Post{
		Message: fmt.Sprintf(msg(lang, "list.title"), len(reminders)),
	}
	model.ParseSlackAttachment(post, attachments)

	if err := p.client.Post.DM(p.botUserID, userID, post); err != nil {
		p.client.Log.Error("Failed to send the reminder list", "user_id", userID, "err", err)
		return ephemeral(msg(lang, "list.sendFailed"))
	}

	// Saying "sent to your DM" inside that very DM is noise.
	if p.isBotDM(userID, channelID) {
		return &model.CommandResponse{}
	}

	return ephemeral(msg(lang, "list.sent"))
}

// isBotDM reports whether the channel is the user's DM with the plugin's bot.
func (p *Plugin) isBotDM(userID, channelID string) bool {
	channel, err := p.client.Channel.Get(channelID)
	if err != nil {
		return false
	}

	return channel.Type == model.ChannelTypeDirect &&
		channel.Name == model.GetDMNameFromIds(p.botUserID, userID)
}

// deleteReminderText removes one reminder.
func (p *Plugin) deleteReminderText(userID, reminderID string) string {
	lang := p.langFor(userID)

	if reminderID == "" {
		return msg(lang, "delete.which")
	}

	r, err := p.kvstore.GetReminder(userID, reminderID)
	if err != nil {
		if errors.Is(err, reminder.ErrNotFound) {
			return fmt.Sprintf(msg(lang, "notFound"), reminderID)
		}
		p.client.Log.Error("Failed to look up a reminder for deletion", "user_id", userID, "err", err)
		return msg(lang, "failed")
	}

	if err := p.kvstore.DeleteReminder(userID, reminderID); err != nil {
		p.client.Log.Error("Failed to delete a reminder", "user_id", userID, "reminder_id", reminderID, "err", err)
		return msg(lang, "failed")
	}

	return fmt.Sprintf(msg(lang, "deleted"), r.Message)
}

// setPausedText pauses or resumes a reminder.
func (p *Plugin) setPausedText(userID, reminderID string, paused bool) string {
	lang := p.langFor(userID)

	verb := "resume"
	if paused {
		verb = "pause"
	}

	if reminderID == "" {
		return fmt.Sprintf(msg(lang, "which"), verb)
	}

	r, err := p.kvstore.GetReminder(userID, reminderID)
	if err != nil {
		if errors.Is(err, reminder.ErrNotFound) {
			return fmt.Sprintf(msg(lang, "notFound"), reminderID)
		}
		p.client.Log.Error("Failed to look up a reminder", "user_id", userID, "err", err)
		return msg(lang, "failed")
	}

	if r.Paused == paused {
		if paused {
			return fmt.Sprintf(msg(lang, "paused.already"), escapeInline(r.Message), r.ID)
		}
		return fmt.Sprintf(msg(lang, "resumed.already"), escapeInline(r.Message), p.formatRunAt(r, lang))
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
		return msg(lang, "failed")
	}

	if paused {
		return fmt.Sprintf(msg(lang, "paused"), escapeInline(r.Message), r.ID)
	}

	return fmt.Sprintf(msg(lang, "resumed"), escapeInline(r.Message), p.formatRunAt(r, lang))
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
func (p *Plugin) formatRunAt(r *reminder.Reminder, lang reminder.Lang) string {
	if r.Paused {
		return msg(lang, "state.paused")
	}
	if r.NextRunAt == 0 {
		return msg(lang, "state.never")
	}

	loc := r.Location()
	next := time.UnixMilli(r.NextRunAt).In(loc)
	today := time.Now().In(loc)
	daysAway := daysBetween(today, next)

	if lang == reminder.LangJA {
		clock := next.Format("15:04 MST")
		switch {
		case daysAway == 0:
			return "今日 " + clock
		case daysAway == 1:
			return "明日 " + clock
		case daysAway < 7:
			return japaneseWeekday(next) + "曜 " + clock
		case next.Year() == today.Year():
			return next.Format("1月2日") + " " + clock
		default:
			return next.Format("2006年1月2日") + " " + clock
		}
	}

	clock := next.Format("15:04 MST")
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

// japaneseWeekday renders the weekday character for a time.
func japaneseWeekday(t time.Time) string {
	return [...]string{"日", "月", "火", "水", "木", "金", "土"}[int(t.Weekday())]
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
