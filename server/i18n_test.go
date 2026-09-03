package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

func TestLangFor(t *testing.T) {
	tests := map[string]reminder.Lang{
		"ja":    reminder.LangJA,
		"ja-JP": reminder.LangJA,
		"JA":    reminder.LangJA,
		"en":    reminder.LangEN,
		"en-US": reminder.LangEN,
		// A locale the plugin has no translation for falls back to English
		// rather than showing the reader nothing.
		"de": reminder.LangEN,
		"fr": reminder.LangEN,
		"":   reminder.LangEN,
	}

	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			tp := newTestPlugin(t)
			tp.api.locale = locale

			assert.Equal(t, want, tp.langFor("user1"))
		})
	}
}

// Every key has to exist in English. A missing one would render as the key
// itself in the middle of a sentence.
func TestEveryMessageHasEnglish(t *testing.T) {
	for key, t2 := range messages {
		assert.NotEmpty(t, t2.en, "message %q has no English text", key)
	}
}

// Japanese is optional per key — a missing one falls back to English rather
// than to an empty string.
func TestMissingJapaneseFallsBackToEnglish(t *testing.T) {
	assert.Equal(t, "hello", text{en: "hello"}.in(reminder.LangJA))
	assert.Equal(t, "こんにちは", text{en: "hello", ja: "こんにちは"}.in(reminder.LangJA))
	assert.Equal(t, "hello", text{en: "hello", ja: "こんにちは"}.in(reminder.LangEN))
}

// An unknown key is a programming error; showing the key beats showing nothing,
// because it is obvious the moment anyone looks at the message.
func TestUnknownMessageKeyShowsTheKey(t *testing.T) {
	assert.Equal(t, "no.such.key", msg(reminder.LangEN, "no.such.key"))
}

// A format string has to take the same arguments in both languages, or the
// translated message renders with %!s(MISSING) in it.
func TestFormatVerbsMatchAcrossLanguages(t *testing.T) {
	for key, t2 := range messages {
		if t2.ja == "" {
			continue
		}

		t.Run(key, func(t *testing.T) {
			assert.Equal(t, countVerbs(t2.en), countVerbs(t2.ja),
				"%q takes %d arguments in English but %d in Japanese",
				key, countVerbs(t2.en), countVerbs(t2.ja))
		})
	}
}

// countVerbs counts printf verbs, ignoring escaped percent signs.
func countVerbs(format string) int {
	count := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			i++
			continue
		}
		count++
	}

	return count
}

// A Japanese user gets the reply in Japanese, including the rendered schedule
// and the next run — the parts most likely to be left in English by accident.
func TestJapaneseUserGetsJapaneseReplies(t *testing.T) {
	tp := newTestPlugin(t)
	tp.api.locale = "ja"
	tp.api.timezone = "Asia/Tokyo"

	got := tp.createReminderText("user1", "毎週月曜 10:00 週次報告")

	assert.Contains(t, got, "週次報告")
	assert.Contains(t, got, "毎週月曜", "the schedule must be described in Japanese")
	assert.NotContains(t, got, "every Monday")
	assert.NotContains(t, got, "Got it")
}

func TestEnglishUserGetsEnglishReplies(t *testing.T) {
	tp := newTestPlugin(t)
	tp.api.locale = "en"
	tp.api.timezone = "Asia/Tokyo"

	got := tp.createReminderText("user1", "every monday at 10:00 weekly report")

	assert.Contains(t, got, "Got it")
	assert.Contains(t, got, "every Monday at 10:00")
}

func TestButtonsAreTranslated(t *testing.T) {
	tp := newTestPlugin(t)

	r := testReminder("user1", "r1", reminder.TimeOfDay{Hour: 9}, time.Now().UnixMilli())

	ja := tp.listActions(r, reminder.LangJA)
	require.Len(t, ja, 2)
	assert.Equal(t, "一時停止", ja[0].Name)
	assert.Equal(t, "削除", ja[1].Name)

	en := tp.listActions(r, reminder.LangEN)
	assert.Equal(t, "Pause", en[0].Name)
	assert.Equal(t, "Delete", en[1].Name)
}

func TestFormatRunAtInJapanese(t *testing.T) {
	tp := newTestPlugin(t)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	noon := time.Now().In(tokyo)
	noon = time.Date(noon.Year(), noon.Month(), noon.Day(), 12, 0, 0, 0, tokyo)

	today := &reminder.Reminder{Timezone: "Asia/Tokyo", NextRunAt: noon.Add(time.Hour).UnixMilli()}
	assert.Contains(t, tp.formatRunAt(today, reminder.LangJA), "今日")

	tomorrow := &reminder.Reminder{Timezone: "Asia/Tokyo", NextRunAt: noon.AddDate(0, 0, 1).UnixMilli()}
	assert.Contains(t, tp.formatRunAt(tomorrow, reminder.LangJA), "明日")

	// The zone abbreviation stays in both languages: it is how a reader spots a
	// reminder that drifted across a daylight-saving change.
	assert.Contains(t, tp.formatRunAt(today, reminder.LangJA), "JST")

	paused := &reminder.Reminder{Timezone: "Asia/Tokyo", Paused: true, NextRunAt: noon.UnixMilli()}
	assert.Equal(t, "一時停止中", tp.formatRunAt(paused, reminder.LangJA))
}

func TestDescribeInJapanese(t *testing.T) {
	tests := map[string]struct {
		schedule reminder.Schedule
		want     string
	}{
		"daily": {
			reminder.Schedule{Kind: reminder.KindDaily, At: reminder.TimeOfDay{Hour: 9}},
			"毎日 9:00 に",
		},
		"weekdays": {
			reminder.Schedule{Kind: reminder.KindWeekdays, At: reminder.TimeOfDay{Hour: 18, Minute: 30}},
			"平日 18:30 に",
		},
		"weekly": {
			reminder.Schedule{
				Kind: reminder.KindWeekly, At: reminder.TimeOfDay{Hour: 10},
				Weekdays: []time.Weekday{time.Monday},
			},
			"毎週月曜 10:00 に",
		},
		"weekly with several days": {
			reminder.Schedule{
				Kind: reminder.KindWeekly, At: reminder.TimeOfDay{Hour: 10},
				Weekdays: []time.Weekday{time.Monday, time.Thursday},
			},
			"毎週月曜・木曜 10:00 に",
		},
		"monthly": {
			reminder.Schedule{Kind: reminder.KindMonthly, At: reminder.TimeOfDay{Hour: 9}, DayOfMonth: 1},
			"毎月1日 9:00 に",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.schedule.Describe(reminder.LangJA))
		})
	}
}

func TestHelpIsTranslated(t *testing.T) {
	tp := newTestPlugin(t)
	tp.api.timezone = "Asia/Tokyo"

	tp.api.locale = "ja"
	ja := tp.helpText("user1")
	assert.Contains(t, ja, "繰り返しリマインダー")
	assert.Contains(t, ja, "毎週月曜")
	assert.Contains(t, ja, "タイムゾーン(Asia/Tokyo)")

	tp.api.locale = "en"
	en := tp.helpText("user1")
	assert.Contains(t, en, "Recurring Reminders")
	assert.Contains(t, en, "timezone (Asia/Tokyo)")
	assert.NotContains(t, en, "繰り返しリマインダー")
}

// The parse failures a first-time user hits are the ones worth translating.
func TestParseErrorsAreTranslated(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantJA    string
		notWantJA string
	}{
		"no schedule": {
			input: "10:00 週次報告", wantJA: "周期・時刻・本文の順", notWantJA: "Couldn't work out",
		},
		"no time": {
			input: "毎週月曜 週次報告", wantJA: "時刻が読み取れませんでした", notWantJA: "The repeat is clear",
		},
		"no message": {
			input: "毎週月曜 10:00", wantJA: "内容がありません", notWantJA: "That's when",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tp := newTestPlugin(t)
			tp.api.locale = "ja"

			got := tp.createReminderText("user1", tc.input)
			assert.Contains(t, got, tc.wantJA)
			assert.NotContains(t, got, tc.notWantJA)
		})
	}
}

// Errors built around the offending text stay in English, and must still be
// readable rather than becoming an empty string.
func TestUntranslatedParseErrorsStillRead(t *testing.T) {
	tp := newTestPlugin(t)
	tp.api.locale = "ja"

	got := tp.createReminderText("user1", "毎日 25:00 無理")

	assert.Contains(t, got, "25:00")
	assert.Contains(t, got, "/recurring help")
}
