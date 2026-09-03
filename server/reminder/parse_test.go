package reminder

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecurring(t *testing.T) {
	tests := []struct {
		input       string
		wantKind    Kind
		wantAt      TimeOfDay
		wantDays    []time.Weekday
		wantDayOfMo int
		wantMessage string
	}{
		// Japanese
		{
			input: "毎週月曜 10:00 週次報告", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "週次報告",
		},
		{
			input: "毎週金曜日 18:30 週報を出す", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 18, Minute: 30}, wantDays: []time.Weekday{time.Friday},
			wantMessage: "週報を出す",
		},
		{
			input: "毎日 9:00 朝会", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "朝会",
		},
		{
			input: "毎朝9時 ストレッチ", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "ストレッチ",
		},
		{
			input: "毎日9時半 水を飲む", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9, Minute: 30}, wantMessage: "水を飲む",
		},
		{
			input: "毎晩 22時30分 日記", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 22, Minute: 30}, wantMessage: "日記",
		},
		{
			input: "平日 18:00 退勤", wantKind: KindWeekdays,
			wantAt: TimeOfDay{Hour: 18}, wantMessage: "退勤",
		},
		{
			input: "毎月1日 9:00 経費精算", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 9}, wantDayOfMo: 1, wantMessage: "経費精算",
		},
		{
			input: "毎月25日 10:00 請求書", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 10}, wantDayOfMo: 25, wantMessage: "請求書",
		},
		{
			// Full-width digits and colon, as a Japanese IME produces them.
			input: "毎日　９：００　朝会", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "朝会",
		},

		// English
		{
			input: "every monday 10:00 weekly report", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "weekly report",
		},
		{
			input: "every monday at 10:00 weekly report", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "weekly report",
		},
		{
			input: "mondays 9am stand-up", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 9}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "stand-up",
		},
		{
			input: "daily 9:00 stand-up", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "stand-up",
		},
		{
			input: "every day at 9am stand-up", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 9}, wantMessage: "stand-up",
		},
		{
			input: "every day at 9pm wind down", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 21}, wantMessage: "wind down",
		},
		{
			input: "weekdays 18:00 log off", wantKind: KindWeekdays,
			wantAt: TimeOfDay{Hour: 18}, wantMessage: "log off",
		},
		{
			input: "every weekday 18:00 log off", wantKind: KindWeekdays,
			wantAt: TimeOfDay{Hour: 18}, wantMessage: "log off",
		},
		{
			input: "monthly on the 1st 9:00 expenses", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 9}, wantDayOfMo: 1, wantMessage: "expenses",
		},
		{
			input: "every month on the 25th 10:00 invoices", wantKind: KindMonthly,
			wantAt: TimeOfDay{Hour: 10}, wantDayOfMo: 25, wantMessage: "invoices",
		},
		{
			// Case should not matter.
			input: "EVERY MONDAY 10:00 Weekly Report", wantKind: KindWeekly,
			wantAt: TimeOfDay{Hour: 10}, wantDays: []time.Weekday{time.Monday},
			wantMessage: "Weekly Report",
		},
		{
			// Midnight and noon are the meridiem edge cases.
			input: "daily 12am midnight check", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 0}, wantMessage: "midnight check",
		},
		{
			input: "daily 12pm lunch", wantKind: KindDaily,
			wantAt: TimeOfDay{Hour: 12}, wantMessage: "lunch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			schedule, message, err := ParseRecurring(tc.input)
			require.NoError(t, err)

			assert.Equal(t, tc.wantKind, schedule.Kind)
			assert.Equal(t, tc.wantAt, schedule.At)
			assert.Equal(t, tc.wantMessage, message)

			if tc.wantDays != nil {
				assert.Equal(t, tc.wantDays, schedule.Weekdays)
			}
			if tc.wantDayOfMo != 0 {
				assert.Equal(t, tc.wantDayOfMo, schedule.DayOfMonth)
			}

			assert.NoError(t, schedule.Validate(), "parser produced a schedule that fails validation")
		})
	}
}

// "every weekday" must not be swallowed by the "every day" rule.
func TestParseRecurringPrefersWeekdaysOverDaily(t *testing.T) {
	schedule, _, err := ParseRecurring("every weekday 09:00 stand-up")
	require.NoError(t, err)
	assert.Equal(t, KindWeekdays, schedule.Kind)
}

func TestParseRecurringErrors(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantErr error
	}{
		"empty":                {input: "", wantErr: ErrNoSchedule},
		"blank":                {input: "   ", wantErr: ErrNoSchedule},
		"no recurrence phrase": {input: "10:00 weekly report", wantErr: ErrNoSchedule},
		"schedule but no message": {
			input: "毎週月曜 10:00", wantErr: ErrNoMessage,
		},
		"english schedule but no message": {
			input: "every monday at 10:00", wantErr: ErrNoMessage,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := ParseRecurring(tc.input)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestParseRecurringRejectsImpossibleTimes(t *testing.T) {
	for _, input := range []string{
		"毎日 25:00 無理",
		"毎日 10:99 無理",
		"daily 25:00 nope",
		"daily 13pm nope",
		"毎日 25時 無理",
	} {
		t.Run(input, func(t *testing.T) {
			_, _, err := ParseRecurring(input)
			assert.Error(t, err)
		})
	}
}

func TestParseRecurringRejectsMissingTime(t *testing.T) {
	_, _, err := ParseRecurring("毎週月曜 週次報告")
	assert.Error(t, err)
}

func TestParseRecurringRejectsOverlongMessage(t *testing.T) {
	long := make([]byte, MaxMessageLength+1)
	for i := range long {
		long[i] = 'a'
	}

	_, _, err := ParseRecurring("daily 9:00 " + string(long))
	assert.Error(t, err)
}

func TestDescribe(t *testing.T) {
	tests := map[string]struct {
		schedule Schedule
		want     string
	}{
		"daily": {
			schedule: Schedule{Kind: KindDaily, At: TimeOfDay{Hour: 9}},
			want:     "every day at 09:00",
		},
		"weekdays": {
			schedule: Schedule{Kind: KindWeekdays, At: TimeOfDay{Hour: 18, Minute: 30}},
			want:     "every weekday at 18:30",
		},
		"weekly": {
			schedule: Schedule{Kind: KindWeekly, At: TimeOfDay{Hour: 10}, Weekdays: []time.Weekday{time.Monday}},
			want:     "every Monday at 10:00",
		},
		"weekly with several days": {
			schedule: Schedule{
				Kind: KindWeekly, At: TimeOfDay{Hour: 10},
				Weekdays: []time.Weekday{time.Monday, time.Thursday},
			},
			want: "every Monday, Thursday at 10:00",
		},
		"monthly 1st": {
			schedule: Schedule{Kind: KindMonthly, At: TimeOfDay{Hour: 9}, DayOfMonth: 1},
			want:     "on the 1st of every month at 09:00",
		},
		"monthly 22nd": {
			schedule: Schedule{Kind: KindMonthly, At: TimeOfDay{Hour: 9}, DayOfMonth: 22},
			want:     "on the 22nd of every month at 09:00",
		},
		"monthly 11th is not 11st": {
			schedule: Schedule{Kind: KindMonthly, At: TimeOfDay{Hour: 9}, DayOfMonth: 11},
			want:     "on the 11th of every month at 09:00",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.schedule.Describe())
		})
	}
}

// Anything the parser accepts must round trip into a description without
// tripping over an unhandled kind.
func TestParsedSchedulesAllDescribe(t *testing.T) {
	for _, input := range []string{
		"毎日 9:00 a",
		"平日 9:00 a",
		"毎週月曜 9:00 a",
		"毎月3日 9:00 a",
	} {
		schedule, _, err := ParseRecurring(input)
		require.NoError(t, err)
		assert.NotEqual(t, "unknown schedule", schedule.Describe())
	}
}

// "every weekdays" used to fail: the alternation matched "every weekday" and
// then the \b assertion failed against the trailing "s".
func TestParseRecurringAcceptsPluralWeekdays(t *testing.T) {
	for _, input := range []string{
		"every weekday 09:00 stand-up",
		"every weekdays 09:00 stand-up",
		"weekday 09:00 stand-up",
		"weekdays 09:00 stand-up",
		"on weekdays 09:00 stand-up",
	} {
		t.Run(input, func(t *testing.T) {
			schedule, message, err := ParseRecurring(input)
			require.NoError(t, err)
			assert.Equal(t, KindWeekdays, schedule.Kind)
			assert.Equal(t, "stand-up", message)
		})
	}
}

// Matching is case-insensitive via (?i) rather than by lower-casing the input,
// because ToLower does not preserve byte offsets. U+212A KELVIN SIGN lower-cases
// to a one-byte "k"; using an offset from the lower-cased string would slice the
// original input in the wrong place and mangle the message.
func TestParseRecurringHandlesLengthChangingCaseFolding(t *testing.T) {
	// "weeKdays" with U+212A in place of the K.
	schedule, message, err := ParseRecurring("weeKdays 10:00 hello")
	require.NoError(t, err)
	assert.Equal(t, KindWeekdays, schedule.Kind)
	assert.Equal(t, "hello", message)
}

// Message length is counted in runes, so a Japanese message gets the full
// advertised allowance rather than a third of it.
func TestParseRecurringCountsMessageLengthInRunes(t *testing.T) {
	// 500 Japanese characters is 1500 bytes but well under the rune limit.
	long := strings.Repeat("あ", 500)

	_, message, err := ParseRecurring("毎日 9:00 " + long)
	require.NoError(t, err)
	assert.Equal(t, long, message)

	tooLong := strings.Repeat("あ", MaxMessageLength+1)
	_, _, err = ParseRecurring("毎日 9:00 " + tooLong)
	assert.Error(t, err)
}

// A trailing am/pm after a 24-hour style time used to be left behind: "9:00am x"
// put "am" at the head of the message, and "9:00pm x" silently meant 09:00
// rather than 21:00 — the wrong time, with no error to show for it.
func TestParseRecurringHandlesMeridiemAfterColonTime(t *testing.T) {
	tests := map[string]struct {
		input       string
		wantHour    int
		wantMinute  int
		wantMessage string
	}{
		"am attached":     {"every day at 9:00am brush teeth", 9, 0, "brush teeth"},
		"am spaced":       {"every day at 9:00 am brush teeth", 9, 0, "brush teeth"},
		"pm attached":     {"daily 9:00pm wind down", 21, 0, "wind down"},
		"pm with minutes": {"daily 6:30pm gym", 18, 30, "gym"},
		"noon":            {"daily 12:00pm lunch", 12, 0, "lunch"},
		"midnight":        {"daily 12:00am backup", 0, 0, "backup"},
		"uppercase":       {"daily 9:00PM wind down", 21, 0, "wind down"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			schedule, message, err := ParseRecurring(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantHour, schedule.At.Hour)
			assert.Equal(t, tc.wantMinute, schedule.At.Minute)
			assert.Equal(t, tc.wantMessage, message)
		})
	}
}

// A bare hour is accepted after an explicit "at", which is how people write it.
func TestParseRecurringAcceptsBareHourAfterAt(t *testing.T) {
	schedule, message, err := ParseRecurring("every day at 9 stand-up")
	require.NoError(t, err)
	assert.Equal(t, 9, schedule.At.Hour)
	assert.Equal(t, 0, schedule.At.Minute)
	assert.Equal(t, "stand-up", message)

	schedule, message, err = ParseRecurring("every monday at 10 weekly report")
	require.NoError(t, err)
	assert.Equal(t, 10, schedule.At.Hour)
	assert.Equal(t, "weekly report", message)
}

// Without an "at" the leading number belongs to the message, not the clock.
func TestParseRecurringDoesNotEatALeadingNumberAsTheHour(t *testing.T) {
	_, _, err := ParseRecurring("every day 3 coffees")
	assert.ErrorIs(t, err, ErrNoTime)
}

func TestParseRecurringRejectsImpossibleMeridiemHour(t *testing.T) {
	for _, input := range []string{"daily 13pm nope", "daily 0am nope", "daily 13:00pm nope"} {
		t.Run(input, func(t *testing.T) {
			_, _, err := ParseRecurring(input)
			assert.Error(t, err)
		})
	}
}

// Every parse error has to name what is missing and show something that works,
// because a failed reminder is how most people meet this plugin.
func TestParseErrorsShowAWorkingExample(t *testing.T) {
	for name, err := range map[string]error{
		"no schedule": ErrNoSchedule,
		"no time":     ErrNoTime,
		"no message":  ErrNoMessage,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, err.Error(), "`", "error should quote an example the user can copy")
		})
	}
}
