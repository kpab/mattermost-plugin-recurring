package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

func TestParseCommand(t *testing.T) {
	tests := map[string]struct {
		raw        string
		wantAction subcommand
		wantArg    string
	}{
		"bare trigger shows help": {
			raw: "/recurring", wantAction: subcommandHelp,
		},
		"help verb": {
			raw: "/recurring help", wantAction: subcommandHelp,
		},
		"list verb": {
			raw: "/recurring list", wantAction: subcommandList,
		},
		"delete with id": {
			raw: "/recurring delete abc123", wantAction: subcommandDelete, wantArg: "abc123",
		},
		"delete without id": {
			raw: "/recurring delete", wantAction: subcommandDelete,
		},
		"pause": {
			raw: "/recurring pause abc123", wantAction: subcommandPause, wantArg: "abc123",
		},
		"resume": {
			raw: "/recurring resume abc123", wantAction: subcommandResume, wantArg: "abc123",
		},
		"stop is an alias for pause": {
			raw: "/recurring stop abc123", wantAction: subcommandPause, wantArg: "abc123",
		},
		"start is an alias for resume": {
			raw: "/recurring start abc123", wantAction: subcommandResume, wantArg: "abc123",
		},
		"remove is an alias for delete": {
			raw: "/recurring remove abc123", wantAction: subcommandDelete, wantArg: "abc123",
		},
		"rm is an alias for delete": {
			raw: "/recurring rm abc123", wantAction: subcommandDelete, wantArg: "abc123",
		},
		"verbs are case insensitive": {
			raw: "/recurring LIST", wantAction: subcommandList,
		},
		"creation is the default": {
			raw:        "/recurring every monday at 10:00 weekly report",
			wantAction: subcommandCreate, wantArg: "every monday at 10:00 weekly report",
		},
		"japanese creation": {
			raw:        "/recurring 毎週月曜 10:00 週次報告",
			wantAction: subcommandCreate, wantArg: "毎週月曜 10:00 週次報告",
		},
		// A reminder whose text happens to start with a verb must not be
		// mistaken for that verb.
		"reminder starting with the word list": {
			raw:        "/recurring list 9:00 は使えない",
			wantAction: subcommandCreate, wantArg: "list 9:00 は使えない",
		},
		"reminder starting with the word help": {
			raw:        "/recurring help daily 9:00 someone",
			wantAction: subcommandCreate, wantArg: "help daily 9:00 someone",
		},
		"extra whitespace is ignored": {
			raw: "   /recurring    list   ", wantAction: subcommandList,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			action, arg := parseCommand(tc.raw)

			assert.Equal(t, tc.wantAction, action)
			assert.Equal(t, tc.wantArg, arg)
		})
	}
}

func TestEscapeInline(t *testing.T) {
	// A newline in the message would otherwise break the surrounding list.
	assert.Equal(t, "a b", escapeInline("a\nb"))
	assert.Equal(t, "a b", escapeInline("a\r\nb"))
	assert.Equal(t, "nothing to escape", escapeInline("nothing to escape"))
}

func TestCapitalise(t *testing.T) {
	assert.Equal(t, "Could not understand", capitalise("could not understand"))
	assert.Equal(t, "", capitalise(""))
	assert.Equal(t, "Already capital", capitalise("Already capital"))
	// Must not corrupt a multi-byte first character.
	assert.Equal(t, "毎日のリマインダー", capitalise("毎日のリマインダー"))
}

func TestFormatRunAt(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	p := &Plugin{}
	now := time.Now().In(tokyo)

	at := func(d time.Duration) *reminder.Reminder {
		return &reminder.Reminder{
			Timezone:  "Asia/Tokyo",
			NextRunAt: now.Add(d).UnixMilli(),
		}
	}

	t.Run("a paused reminder has no next run to show", func(t *testing.T) {
		r := at(48 * time.Hour)
		r.Paused = true

		assert.Equal(t, "paused", p.formatRunAt(r),
			"showing a next run for a paused reminder contradicts the pause")
	})

	t.Run("a spent reminder says so", func(t *testing.T) {
		assert.Equal(t, "never again", p.formatRunAt(&reminder.Reminder{Timezone: "Asia/Tokyo"}))
	})

	t.Run("near dates are named", func(t *testing.T) {
		// Anchored to noon so adding hours cannot roll over a day boundary.
		noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, tokyo)

		today := &reminder.Reminder{Timezone: "Asia/Tokyo", NextRunAt: noon.Add(time.Hour).UnixMilli()}
		assert.Contains(t, p.formatRunAt(today), "Today at")

		tomorrow := &reminder.Reminder{Timezone: "Asia/Tokyo", NextRunAt: noon.AddDate(0, 0, 1).UnixMilli()}
		assert.Contains(t, p.formatRunAt(tomorrow), "Tomorrow at")
	})

	t.Run("the timezone is always shown", func(t *testing.T) {
		// It is how a user notices a reminder drifted across a DST change.
		assert.Contains(t, p.formatRunAt(at(3*time.Hour)), "JST")
	})
}
