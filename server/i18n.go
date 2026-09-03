package main

import (
	"strings"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// Translations are a small hand-written table rather than a message catalogue.
// There are two languages and a few dozen strings, and every one of them is
// interpolated in a different shape — a catalogue would add a build step and a
// file format without removing anything.
//
// The language follows the reader's Mattermost locale, so `/recurring list` and
// `help` answer in the same language as the rest of their client. Anything the
// locale does not cover falls back to English.

// langFor returns the language to answer this user in.
func (p *Plugin) langFor(userID string) reminder.Lang {
	user, err := p.client.User.Get(userID)
	if err != nil {
		return reminder.LangEN
	}

	// Locales arrive as "ja" or as a tag like "ja-JP".
	if strings.HasPrefix(strings.ToLower(user.Locale), "ja") {
		return reminder.LangJA
	}

	return reminder.LangEN
}

// text holds one string in both languages.
type text struct {
	en string
	ja string
}

// in picks the wording for a language.
func (t text) in(lang reminder.Lang) string {
	if lang == reminder.LangJA && t.ja != "" {
		return t.ja
	}

	return t.en
}

var messages = map[string]text{
	"list.title": {
		en: "**Your reminders** (%d)",
		ja: "**リマインダー** (%d件)",
	},
	"list.empty": {
		en: "You have no reminders yet. Create one like this:\n" +
			"`/recurring every monday at 10:00 weekly report`\n" +
			"More examples: `/recurring help`",
		ja: "リマインダーはまだありません。こんなふうに作れます:\n" +
			"`/recurring 毎週月曜 10:00 週次報告`\n" +
			"ほかの例: `/recurring help`",
	},
	"list.sent": {
		en: "Sent your reminders to your **Recurring Reminders** direct message.",
		ja: "**Recurring Reminders** とのダイレクトメッセージに一覧を送りました。",
	},
	"list.failed": {
		en: "Something went wrong reading your reminders. Please try again.",
		ja: "リマインダーの読み込みに失敗しました。もう一度お試しください。",
	},
	"list.sendFailed": {
		en: "Something went wrong sending your reminders. Please try again.",
		ja: "一覧の送信に失敗しました。もう一度お試しください。",
	},
	"created": {
		en: "Got it — I'll remind you **%s** %s.\nNext: **%s**\nTo cancel: `/recurring delete %s`",
		ja: "了解しました。**%s** を%sお知らせします。\n次回: **%s**\n取り消す: `/recurring delete %s`",
	},
	"created.never": {
		en: "That reminder would never fire.",
		ja: "そのリマインダーは一度も鳴りません。",
	},
	"created.failed": {
		en: "Something went wrong saving that reminder. Please try again.",
		ja: "リマインダーの保存に失敗しました。もう一度お試しください。",
	},
	"created.tooMany": {
		en: ". Delete one with `/recurring delete <id>`, then try again.",
		ja: "。`/recurring delete <id>` でどれか消してから、もう一度お試しください。",
	},
	"deleted": {
		en: "Deleted **%s**.",
		ja: "**%s** を削除しました。",
	},
	"delete.which": {
		en: "Which one? Use `/recurring list` to see your reminders — each one comes with the command to remove it.",
		ja: "どれを消しますか? `/recurring list` で一覧を出すと、消すためのボタンが付いています。",
	},
	"notFound": {
		en: "No reminder with ID `%s`. Use `/recurring list` to see yours.",
		ja: "ID `%s` のリマインダーはありません。`/recurring list` で一覧を確認してください。",
	},
	"failed": {
		en: "Something went wrong. Please try again.",
		ja: "エラーが発生しました。もう一度お試しください。",
	},
	"paused": {
		en: "Paused **%s**. Resume it with `/recurring resume %s`.",
		ja: "**%s** を一時停止しました。`/recurring resume %s` で再開できます。",
	},
	"paused.already": {
		en: "**%s** is already paused. Resume it with `/recurring resume %s`.",
		ja: "**%s** はすでに一時停止しています。`/recurring resume %s` で再開できます。",
	},
	"resumed": {
		en: "Resumed **%s**. Next: **%s**",
		ja: "**%s** を再開しました。次回: **%s**",
	},
	"resumed.already": {
		en: "**%s** is already running. Next: **%s**",
		ja: "**%s** は動いています。次回: **%s**",
	},
	"which": {
		en: "Which one? Use `/recurring list` to see your reminders, then `/recurring %s <id>`.",
		ja: "どれですか? `/recurring list` で一覧を出してから `/recurring %s <id>` を実行してください。",
	},
	"button.snooze": {
		en: "Snooze %s",
		ja: "%s後に再通知",
	},
	"button.stop": {
		en: "Stop this reminder",
		ja: "このリマインダーを停止",
	},
	"button.pause": {
		en: "Pause",
		ja: "一時停止",
	},
	"button.resume": {
		en: "Resume",
		ja: "再開",
	},
	"button.delete": {
		en: "Delete",
		ja: "削除",
	},
	"snooze.15": {
		en: "15 min",
		ja: "15分",
	},
	"snooze.60": {
		en: "1 hour",
		ja: "1時間",
	},
	"action.snoozed": {
		en: "⏰ **%s**\n_snoozed until %s_",
		ja: "⏰ **%s**\n_%s に再通知します_",
	},
	"action.paused": {
		en: "⏰ **%s**\n_paused · resume it with_ `/recurring resume %s`",
		ja: "⏰ **%s**\n_一時停止しました · 再開は_ `/recurring resume %s`",
	},
	"action.resumed": {
		en: "⏰ **%s**\n_resumed · next %s_",
		ja: "⏰ **%s**\n_再開しました · 次回 %s_",
	},
	"action.deleted": {
		en: "~~%s~~ _deleted_",
		ja: "~~%s~~ _削除しました_",
	},
	"action.gone": {
		en: "That reminder is gone — it looks like it was already deleted.",
		ja: "そのリマインダーはもうありません。すでに削除されたようです。",
	},
	"delivered": {
		en: "⏰ **%s**\n_%s_",
		ja: "⏰ **%s**\n_%s_",
	},
	"next": {
		en: "next %s",
		ja: "次回 %s",
	},
	"state.paused": {
		en: "paused",
		ja: "一時停止中",
	},
	"state.never": {
		en: "never again",
		ja: "以後鳴りません",
	},
	"parse.noSchedule": {
		en: "Couldn't work out how often to repeat this. Put the repeat first, then the time, then the message — like `every monday at 10:00 weekly report`",
		ja: "どのくらいの間隔で繰り返すか読み取れませんでした。周期・時刻・本文の順に書いてください。例: `毎週月曜 10:00 週次報告`",
	},
	"parse.noTime": {
		en: "The repeat is clear, but not the time. Add one like `10:00`, `9am` or `18:30`",
		ja: "繰り返しは分かりましたが、時刻が読み取れませんでした。`10:00` `9時` `9時半` のように書いてください",
	},
	"parse.noMessage": {
		en: "That's when, but not what. Put the message after the time — like `every day at 09:00 stand-up`",
		ja: "いつ鳴らすかは分かりましたが、内容がありません。時刻のあとに本文を書いてください。例: `毎日 9:00 朝会`",
	},
	"parse.retry": {
		en: "%s\n\nTry `/recurring help` for examples.",
		ja: "%s\n\n`/recurring help` に例があります。",
	},
	"help.timezone": {
		en: "%s\n\nReminders arrive as a direct message, in your own timezone (%s).",
		ja: "%s\n\nリマインダーはダイレクトメッセージで、あなたのタイムゾーン(%s)で届きます。",
	},
	"help.timezoneUnknown": {
		en: "%s\n\nReminders arrive as a direct message, in your own timezone.",
		ja: "%s\n\nリマインダーはダイレクトメッセージで、あなたのタイムゾーンで届きます。",
	},
}

// msg returns a message in the given language.
func msg(lang reminder.Lang, key string) string {
	t, ok := messages[key]
	if !ok {
		// A missing key is a programming error; showing the key beats showing
		// an empty message, and it is obvious in a screenshot.
		return key
	}

	return t.in(lang)
}
