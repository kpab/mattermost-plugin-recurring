package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestEscapeTableCell(t *testing.T) {
	// A pipe would split the markdown table into another column, a newline
	// into another row.
	assert.Equal(t, `a \| b`, escapeTableCell("a | b"))
	assert.Equal(t, "a b", escapeTableCell("a\nb"))
	assert.Equal(t, "nothing to escape", escapeTableCell("nothing to escape"))
}

func TestCapitalise(t *testing.T) {
	assert.Equal(t, "Could not understand", capitalise("could not understand"))
	assert.Equal(t, "", capitalise(""))
	assert.Equal(t, "Already capital", capitalise("Already capital"))
	// Must not corrupt a multi-byte first character.
	assert.Equal(t, "毎日のリマインダー", capitalise("毎日のリマインダー"))
}
